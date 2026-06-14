package validation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	"github.com/iicpc/track3/telemetry-engine/pkg/models"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

// Options configures the validation Engine.
type Options struct {
	Brokers       []string
	UseTLS        bool
	TickSize      float64
	FlushInterval time.Duration // how often to publish updated verdicts
}

// Engine consumes the ordered order-lifecycle stream and maintains one
// Validator per (run, submission). Like the stream processor it uses a single
// event-loop goroutine so each Validator is lock-free.
//
// ORDERING REQUIREMENT: the ingestion service keys every order event by
// run:submission, so all of a submission's orders land on ONE partition and are
// delivered to this engine IN ORDER. Deterministic book replay depends on it.
type Engine struct {
	opt      Options
	log      *logpkg.Logger
	producer *kafka.Producer
	pg       *store.Postgres

	validators map[string]*Validator
	dirty      map[string]bool
}

// NewEngine wires the validation engine.
func NewEngine(opt Options, producer *kafka.Producer, pg *store.Postgres, log *logpkg.Logger) *Engine {
	if opt.FlushInterval <= 0 {
		opt.FlushInterval = 2 * time.Second
	}
	if opt.TickSize <= 0 {
		opt.TickSize = 0.01
	}
	return &Engine{
		opt:        opt,
		log:        log,
		producer:   producer,
		pg:         pg,
		validators: map[string]*Validator{},
		dirty:      map[string]bool{},
	}
}

func (e *Engine) validatorFor(runID, submissionID string) *Validator {
	key := runID + ":" + submissionID
	v := e.validators[key]
	if v == nil {
		v = NewValidator(runID, submissionID, e.opt.TickSize)
		e.validators[key] = v
	}
	return v
}

type inbound struct {
	msg kafka.Message
	env *events.Envelope
}

// Run consumes order events until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	c := kafka.NewConsumer(e.opt.Brokers, events.GroupValidation, events.TopicOrders, e.opt.UseTLS)
	defer c.Close()

	in := make(chan inbound, 4096)
	go func() {
		for {
			msg, err := c.Read(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				e.log.Error("validation read", logpkg.F("err", err.Error()))
				time.Sleep(200 * time.Millisecond)
				continue
			}
			env, err := events.UnmarshalEnvelope(msg.Value)
			if err != nil {
				_ = c.Commit(ctx, msg)
				continue
			}
			select {
			case <-ctx.Done():
				return
			case in <- inbound{msg: msg, env: env}:
			}
		}
	}()

	ticker := time.NewTicker(e.opt.FlushInterval)
	defer ticker.Stop()
	e.log.Info("validation engine started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m := <-in:
			e.apply(m.env)
			_ = c.Commit(ctx, m.msg)
		case <-ticker.C:
			e.flush(ctx)
		}
	}
}

func (e *Engine) apply(env *events.Envelope) {
	key := env.RunID + ":" + env.SubmissionID
	switch env.Type {
	case events.TypeOrderSubmitted:
		if o, err := env.AsOrderSubmitted(); err == nil {
			e.validatorFor(env.RunID, env.SubmissionID).OnSubmitted(o)
			e.dirty[key] = true
		}
	case events.TypeOrderAck:
		if a, err := env.AsOrderAck(); err == nil {
			e.validatorFor(env.RunID, env.SubmissionID).OnAck(a)
			e.dirty[key] = true
		}
	case events.TypeOrderFilled:
		if f, err := env.AsOrderFilled(); err == nil {
			e.validatorFor(env.RunID, env.SubmissionID).OnFilled(f)
			e.dirty[key] = true
		}
	case events.TypeOrderCancelled:
		if cl, err := env.AsOrderCancelled(); err == nil {
			e.validatorFor(env.RunID, env.SubmissionID).OnCancelled(cl)
			e.dirty[key] = true
		}
	}
}

func (e *Engine) flush(ctx context.Context) {
	for key, dirty := range e.dirty {
		if !dirty {
			continue
		}
		e.dirty[key] = false
		v := e.validators[key]
		res := v.Result()
		e.publish(ctx, res)
		e.persist(ctx, res)
	}
}

func (e *Engine) publish(ctx context.Context, res models.ValidationResult) {
	body, err := json.Marshal(res)
	if err != nil {
		return
	}
	key := res.RunID + ":" + res.SubmissionID
	if err := e.producer.PublishRaw(ctx, events.TopicValidationResult, key, body); err != nil {
		e.log.Error("publish validation", logpkg.F("err", err.Error()))
	}
}

func (e *Engine) persist(ctx context.Context, res models.ValidationResult) {
	if e.pg == nil {
		return
	}
	findings, _ := json.Marshal(res.RecentFindings)
	byRule, _ := json.Marshal(res.ViolationsByRule)
	const q = `
INSERT INTO validation_results
  (run_id, submission_id, updated_at, orders_checked, trades_checked,
   violations, violations_by_rule, correctness_score, recent_findings)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (run_id, submission_id)
DO UPDATE SET
  updated_at        = EXCLUDED.updated_at,
  orders_checked    = EXCLUDED.orders_checked,
  trades_checked    = EXCLUDED.trades_checked,
  violations        = EXCLUDED.violations,
  violations_by_rule = EXCLUDED.violations_by_rule,
  correctness_score = EXCLUDED.correctness_score,
  recent_findings   = EXCLUDED.recent_findings;`
	if _, err := e.pg.Pool.Exec(ctx, q,
		res.RunID, res.SubmissionID, res.UpdatedAt, res.OrdersChecked, res.TradesChecked,
		res.Violations, byRule, res.CorrectnessScore, findings,
	); err != nil {
		e.log.Error("persist validation", logpkg.F("err", err.Error()))
	}
}
