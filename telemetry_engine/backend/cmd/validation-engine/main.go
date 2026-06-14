// Command validation-engine replays each submission's order lifecycle against a
// reference limit-order book and checks matching-engine correctness: price-time
// priority, FIFO, fill quantity, book consistency, and trade matching.
package main

import (
	"github.com/iicpc/track3/telemetry-engine/internal/config"
	"github.com/iicpc/track3/telemetry-engine/internal/runtimex"
	"github.com/iicpc/track3/telemetry-engine/internal/validation"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

func main() {
	cfg := config.Load("validation-engine")
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

	engine := validation.NewEngine(validation.Options{
		Brokers:       cfg.KafkaBrokers,
		UseTLS:        cfg.KafkaTLS,
		FlushInterval: cfg.FlushInterval,
	}, producer, pg, log)

	if err := engine.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("validation engine stopped", logpkg.F("err", err.Error()))
	}
	log.Info("shutting down")
}
