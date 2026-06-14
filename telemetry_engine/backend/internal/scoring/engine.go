package scoring

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	"github.com/iicpc/track3/telemetry-engine/pkg/models"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

// Options configures the scoring Engine.
type Options struct {
	Brokers       []string
	UseTLS        bool
	Scorer        Config
	FlushInterval time.Duration
}

// Engine joins the window-aggregate stream with the validation-result stream
// per submission and emits a composite Score whenever either input changes. It
// computes throughput stability (CoV) itself, online, from the sequence of
// TUMBLING windows it observes (non-overlapping → uncorrelated samples).
type Engine struct {
	opt      Options
	log      *logpkg.Logger
	producer *kafka.Producer
	pg       *store.Postgres
	states   map[string]*subState
}

type subState struct {
	runID, submissionID string

	tps       float64
	p50, p99  int64
	errorRate float64
	correct   float64

	// Welford accumulators over tumbling-window TPS → coefficient of variation.
	n          int64
	mean, m2   float64
	cov        float64
	haveWindow bool
	dirty      bool
}

func NewEngine(opt Options, producer *kafka.Producer, pg *store.Postgres, log *logpkg.Logger) *Engine {
	if opt.FlushInterval <= 0 {
		opt.FlushInterval = time.Second
	}
	return &Engine{opt: opt, log: log, producer: producer, pg: pg, states: map[string]*subState{}}
}

func (e *Engine) stateFor(runID, submissionID string) *subState {
	key := runID + ":" + submissionID
	s := e.states[key]
	if s == nil {
		s = &subState{runID: runID, submissionID: submissionID, correct: 1.0}
		e.states[key] = s
	}
	return s
}

type inbound struct {
	consumer *kafka.Consumer
	msg      kafka.Message
	topic    string
	value    []byte
	runID    string
	subID    string
}

// Run consumes both input topics until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	win := kafka.NewConsumer(e.opt.Brokers, events.GroupScoring, events.TopicWindowAggregates, e.opt.UseTLS)
	val := kafka.NewConsumer(e.opt.Brokers, events.GroupScoring, events.TopicValidationResult, e.opt.UseTLS)
	defer win.Close()
	defer val.Close()

	in := make(chan inbound, 4096)
	go e.consume(ctx, win, events.TopicWindowAggregates, in)
	go e.consume(ctx, val, events.TopicValidationResult, in)

	ticker := time.NewTicker(e.opt.FlushInterval)
	defer ticker.Stop()
	e.log.Info("scoring engine started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m := <-in:
			e.apply(m)
			_ = m.consumer.Commit(ctx, m.msg)
		case <-ticker.C:
			e.flush(ctx)
		}
	}
}

func (e *Engine) consume(ctx context.Context, c *kafka.Consumer, topic string, out chan<- inbound) {
	for {
		msg, err := c.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			e.log.Error("scoring read", logpkg.F("topic", topic, "err", err.Error()))
			time.Sleep(200 * time.Millisecond)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case out <- inbound{consumer: c, msg: msg, topic: topic, value: msg.Value, runID: keyRun(msg.Key), subID: keySub(msg.Key)}:
		}
	}
}

func (e *Engine) apply(m inbound) {
	switch m.topic {
	case events.TopicWindowAggregates:
		var wa models.WindowAggregate
		if json.Unmarshal(m.value, &wa) != nil {
			return
		}
		st := e.stateFor(wa.RunID, wa.SubmissionID)
		st.tps = wa.TPS
		st.p50 = wa.P50US
		st.p99 = wa.P99US
		st.errorRate = wa.ErrorRate
		st.haveWindow = true
		st.dirty = true
		if wa.WindowKind == "tumbling" {
			// Online CoV via Welford over uncorrelated tumbling windows.
			st.n++
			delta := wa.TPS - st.mean
			st.mean += delta / float64(st.n)
			st.m2 += delta * (wa.TPS - st.mean)
			if st.n >= 2 && st.mean > 0 {
				st.cov = math.Sqrt(st.m2/float64(st.n-1)) / st.mean
			}
		}
	case events.TopicValidationResult:
		var vr models.ValidationResult
		if json.Unmarshal(m.value, &vr) != nil {
			return
		}
		st := e.stateFor(vr.RunID, vr.SubmissionID)
		st.correct = vr.CorrectnessScore
		st.dirty = true
	}
}

func (e *Engine) flush(ctx context.Context) {
	for _, st := range e.states {
		if !st.dirty || !st.haveWindow {
			continue
		}
		st.dirty = false
		comp := e.opt.Scorer.Compute(Inputs{
			TPS:         st.tps,
			P99US:       st.p99,
			ErrorRate:   st.errorRate,
			Correctness: st.correct,
			TPSCoV:      st.cov,
		})
		score := models.Score{
			RunID:            st.runID,
			SubmissionID:     st.submissionID,
			ComputedAt:       time.Now().UTC(),
			LatencyScore:     comp.Latency,
			ThroughputScore:  comp.Throughput,
			CorrectnessScore: comp.Correctness,
			StabilityScore:   comp.Stability,
			Composite:        comp.Composite,
			TPS:              st.tps,
			P50US:            st.p50,
			P99US:            st.p99,
			ErrorRate:        st.errorRate,
		}
		e.publish(ctx, score)
		e.persist(ctx, score)
	}
}

func (e *Engine) publish(ctx context.Context, s models.Score) {
	body, err := json.Marshal(s)
	if err != nil {
		return
	}
	key := s.RunID + ":" + s.SubmissionID
	if err := e.producer.PublishRaw(ctx, events.TopicScores, key, body); err != nil {
		e.log.Error("publish score", logpkg.F("err", err.Error()))
	}
}

func (e *Engine) persist(ctx context.Context, s models.Score) {
	if e.pg == nil {
		return
	}
	const q = `
INSERT INTO scores
  (run_id, submission_id, computed_at, latency_score, throughput_score,
   correctness_score, stability_score, composite, tps, p50_us, p99_us, error_rate)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (run_id, submission_id)
DO UPDATE SET
  computed_at       = EXCLUDED.computed_at,
  latency_score     = EXCLUDED.latency_score,
  throughput_score  = EXCLUDED.throughput_score,
  correctness_score = EXCLUDED.correctness_score,
  stability_score   = EXCLUDED.stability_score,
  composite         = EXCLUDED.composite,
  tps               = EXCLUDED.tps,
  p50_us            = EXCLUDED.p50_us,
  p99_us            = EXCLUDED.p99_us,
  error_rate        = EXCLUDED.error_rate;`
	if _, err := e.pg.Pool.Exec(ctx, q,
		s.RunID, s.SubmissionID, s.ComputedAt, s.LatencyScore, s.ThroughputScore,
		s.CorrectnessScore, s.StabilityScore, s.Composite, s.TPS, s.P50US, s.P99US, s.ErrorRate,
	); err != nil {
		e.log.Error("persist score", logpkg.F("err", err.Error()))
	}
}

// keyRun / keySub split a "run:submission" Kafka key.
func keyRun(key []byte) string {
	s := string(key)
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return s[:i]
		}
	}
	return s
}

func keySub(key []byte) string {
	s := string(key)
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return s[i+1:]
		}
	}
	return ""
}
