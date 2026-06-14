// Command scoring-engine joins the window-aggregate and validation-result
// streams per submission and emits a composite Score (latency, throughput,
// correctness, stability) that ranks the leaderboard.
package main

import (
	"github.com/iicpc/track3/telemetry-engine/internal/config"
	"github.com/iicpc/track3/telemetry-engine/internal/runtimex"
	"github.com/iicpc/track3/telemetry-engine/internal/scoring"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

func main() {
	cfg := config.Load("scoring-engine")
	log := logpkg.New(cfg.ServiceName)
	ctx, cancel := runtimex.SignalContext()
	defer cancel()

	producer := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTLS)
	defer producer.Close()

	pg, err := store.NewPostgres(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Warn("postgres unavailable; persistence disabled", logpkg.F("err", err.Error()))
	} else {
		defer pg.Close()
	}

	metrics := runtimex.ServeMetrics(cfg.MetricsAddr, log)
	defer runtimex.Shutdown(metrics)

	scorer := scoring.DefaultConfig()
	scorer.WeightLatency = cfg.WeightLatency
	scorer.WeightThroughput = cfg.WeightThroughput
	scorer.WeightCorrectness = cfg.WeightCorrectness
	scorer.WeightStability = cfg.WeightStability

	engine := scoring.NewEngine(scoring.Options{
		Brokers:       cfg.KafkaBrokers,
		UseTLS:        cfg.KafkaTLS,
		Scorer:        scorer,
		FlushInterval: cfg.FlushInterval,
	}, producer, pg, log)

	if err := engine.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("scoring engine stopped", logpkg.F("err", err.Error()))
	}
	log.Info("shutting down")
}
