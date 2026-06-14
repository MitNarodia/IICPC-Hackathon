package processing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/iicpc/track3/telemetry-engine/internal/percentile"
	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	"github.com/iicpc/track3/telemetry-engine/pkg/models"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

// Options configures a Processor.
type Options struct {
	Brokers       []string
	UseTLS        bool
	Granularity   time.Duration // slot width (resolution)
	WindowSize    time.Duration // sliding window width
	Retention     time.Duration // rolling horizon
	Tumbling      time.Duration // tumbling window width
	FlushInterval time.Duration // how often to emit sliding windows
	// UseBotMetricsHistogram, when true, rebuilds latency windows from Track 2's
	// pre-aggregated BotMetrics histograms instead of from raw acks. Off by
	// default: we drive windows from the raw OrderAck stream and use BotMetrics
	// only to top up the `timeouts` counter (which has no raw event).
	UseBotMetricsHistogram bool
}

// Processor is the stream-processing service. It owns all per-submission state
// on a SINGLE goroutine (the event loop); consumer goroutines only decode and
// forward. This reproduces Track 2's share-nothing model: no locks on the path
// that touches histograms.
type Processor struct {
	opt      Options
	log      *logpkg.Logger
	producer *kafka.Producer
	pg       *store.Postgres
	layout   HistLayout

	states map[string]*submissionState
}

type submissionState struct {
	runID, submissionID string
	agg                 *RollingAggregator
	rolling             *RollingStats
	dirty               bool // received data since last flush
}

// inbound carries a decoded message plus the consumer to commit it on, so the
// event loop can ack only after the sample is folded into state.
type inbound struct {
	consumer *kafka.Consumer
	msg      kafka.Message
	env      *events.Envelope
}

// NewProcessor wires the processor. `pg` may be nil (metrics still flow to
// Kafka; only TimescaleDB persistence is skipped).
func NewProcessor(opt Options, producer *kafka.Producer, pg *store.Postgres, log *logpkg.Logger) *Processor {
	if opt.Granularity <= 0 {
		opt.Granularity = time.Second
	}
	return &Processor{
		opt:      opt,
		log:      log,
		producer: producer,
		pg:       pg,
		layout:   DefaultLayout,
		states:   map[string]*submissionState{},
	}
}

func (p *Processor) stateFor(runID, submissionID string) *submissionState {
	key := runID + ":" + submissionID
	s := p.states[key]
	if s == nil {
		s = &submissionState{
			runID:        runID,
			submissionID: submissionID,
			agg:          NewRollingAggregator(p.opt.Granularity, p.opt.Retention, p.opt.Tumbling, p.layout),
			rolling:      NewRollingStats(runID, submissionID),
		}
		p.states[key] = s
	}
	return s
}

// Run consumes orders + bot metrics + sandbox metrics until ctx is cancelled,
// folding each into per-submission windows and periodically emitting aggregates.
func (p *Processor) Run(ctx context.Context) error {
	orders := kafka.NewConsumer(p.opt.Brokers, events.GroupStreamProcessor, events.TopicOrders, p.opt.UseTLS)
	botm := kafka.NewConsumer(p.opt.Brokers, events.GroupStreamProcessor, events.TopicBotMetrics, p.opt.UseTLS)
	sbx := kafka.NewConsumer(p.opt.Brokers, events.GroupStreamProcessor, events.TopicSandboxMetrics, p.opt.UseTLS)
	defer orders.Close()
	defer botm.Close()
	defer sbx.Close()

	in := make(chan inbound, 4096)
	go p.consume(ctx, orders, in)
	go p.consume(ctx, botm, in)
	go p.consume(ctx, sbx, in)

	ticker := time.NewTicker(p.opt.FlushInterval)
	defer ticker.Stop()

	p.log.Info("stream processor started", logpkg.F(
		"window", p.opt.WindowSize.String(), "tumbling", p.opt.Tumbling.String(),
		"retention", p.opt.Retention.String()))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m := <-in:
			p.handle(ctx, m)
		case <-ticker.C:
			p.flush(ctx)
		}
	}
}

func (p *Processor) consume(ctx context.Context, c *kafka.Consumer, out chan<- inbound) {
	for {
		msg, err := c.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.log.Error("consume read", logpkg.F("err", err.Error()))
			time.Sleep(200 * time.Millisecond)
			continue
		}
		env, err := events.UnmarshalEnvelope(msg.Value)
		if err != nil {
			p.log.Warn("bad envelope", logpkg.F("err", err.Error()))
			_ = c.Commit(ctx, msg) // poison message: skip, don't block the group
			continue
		}
		select {
		case <-ctx.Done():
			return
		case out <- inbound{consumer: c, msg: msg, env: env}:
		}
	}
}

func (p *Processor) handle(ctx context.Context, m inbound) {
	env := m.env
	switch env.Type {
	case events.TypeOrderAck:
		if ack, err := env.AsOrderAck(); err == nil {
			st := p.stateFor(env.RunID, env.SubmissionID)
			at := time.Unix(0, ack.RecvTS)
			lat := ack.AckLatencyUS
			if lat < 0 {
				lat = -1 // no latency sample
			}
			st.agg.Record(at, lat, !ack.Accepted, false)
			st.dirty = true
		}
	case events.TypeBotMetrics:
		if bm, err := env.AsBotMetrics(); err == nil {
			st := p.stateFor(env.RunID, env.SubmissionID)
			at := time.Unix(0, bm.WindowEndTS)
			if p.opt.UseBotMetricsHistogram {
				// Fast path: rebuild the latency window EXACTLY from Track 2's
				// shard histogram, merging across shards in our own ring.
				h := p.decodeHist(bm.HdrBuckets)
				st.agg.RecordPreaggregated(at, h, bm.Transactions, bm.Errors, bm.Timeouts)
			} else {
				// Default: only top up timeouts (no raw event carries them).
				st.agg.RecordPreaggregated(at, nil, 0, 0, bm.Timeouts)
			}
			st.dirty = true
		}
	case events.TypeSandboxMetrics:
		if sm, err := env.AsSandboxMetrics(); err == nil {
			p.log.Info("sandbox_metrics received", logpkg.F(
				"run_id", env.RunID, "submission_id", env.SubmissionID,
				"health", sm.Health, "cpu_mc", fmt.Sprintf("%.1f", sm.CPUMillicores),
				"mem_bytes", fmt.Sprintf("%d", sm.MemoryBytes)))
			p.persistSandboxSample(ctx, env.RunID, env.SubmissionID, sm)
		} else {
			p.log.Warn("bad sandbox_metrics payload", logpkg.F("err", err.Error()))
		}
	default:
		// order_submitted / order_filled / order_cancelled are for the
		// validation engine; the metrics processor ignores them.
	}
	_ = m.consumer.Commit(ctx, m.msg)
}

// decodeHist turns the base64-less raw int64 bucket array (JSON-encoded) back
// into a histogram with our canonical layout.
func (p *Processor) decodeHist(raw []byte) *percentile.Histogram {
	h := p.layout.New()
	if len(raw) == 0 {
		return h
	}
	var counts []int64
	if err := json.Unmarshal(raw, &counts); err != nil {
		return h
	}
	h.LoadCounts(counts)
	return h
}

// flush emits, for every submission that changed since the last tick:
//   - the SLIDING window aggregate (smooth live charts),
//   - any newly-CLOSED tumbling window (authoritative record + stability input),
//   - the updated rolling stats.
func (p *Processor) flush(ctx context.Context) {
	for _, st := range p.states {
		if !st.dirty {
			continue
		}
		st.dirty = false

		// 1) Closed tumbling windows feed stability + durable history.
		if tw, ok := st.agg.ClosedTumbling(); ok {
			st.rolling.ObserveWindow(tw)
			p.emitWindow(ctx, st, tw, "tumbling")
		}

		// 2) Sliding window drives the live dashboard.
		sw := st.agg.Sliding(p.opt.WindowSize)
		p.emitWindow(ctx, st, sw, "sliding")

		// 3) Rolling stats (stability/variance) ride along on the scores topic
		//    input via the window; also persist the snapshot.
		p.persistRolling(ctx, st.rolling.Snapshot())
	}
}

func (p *Processor) emitWindow(ctx context.Context, st *submissionState, a Aggregate, kind string) {
	wa := models.WindowAggregate{
		RunID:        st.runID,
		SubmissionID: st.submissionID,
		WindowStart:  a.Start.UTC(),
		WindowEnd:    a.End.UTC(),
		WindowKind:   kind,
		Transactions: a.Transactions,
		Errors:       a.Errors,
		Timeouts:     a.Timeouts,
		TPS:          a.TPS,
		ErrorRate:    a.ErrorRate,
		P50US:        a.P50US,
		P90US:        a.P90US,
		P99US:        a.P99US,
		MaxUS:        a.MaxUS,
		MeanUS:       a.MeanUS,
		SampleCount:  a.SampleCount,
	}
	body, err := json.Marshal(wa)
	if err != nil {
		return
	}
	key := st.runID + ":" + st.submissionID
	if err := p.producer.PublishRaw(ctx, events.TopicWindowAggregates, key, body); err != nil {
		p.log.Error("publish window", logpkg.F("err", err.Error()))
	}
	p.persistWindow(ctx, wa)
}
