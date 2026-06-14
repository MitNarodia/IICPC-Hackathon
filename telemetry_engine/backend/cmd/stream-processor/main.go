// Command stream-processor turns the raw order/metrics streams into windowed
// aggregates (TPS, error rate, latency percentiles) per (run, submission). It
// owns all state on a single event-loop goroutine — no locks on the hot path.
package main

import (
	"github.com/iicpc/track3/telemetry-engine/internal/config"
	"github.com/iicpc/track3/telemetry-engine/internal/processing"
	"github.com/iicpc/track3/telemetry-engine/internal/runtimex"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

func main() {
	cfg := config.Load("stream-processor")
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

	proc := processing.NewProcessor(processing.Options{
		Brokers:       cfg.KafkaBrokers,
		UseTLS:        cfg.KafkaTLS,
		Granularity:   cfg.SlideInterval,
		WindowSize:    cfg.WindowSize,
		Retention:     cfg.RollingRetention,
		Tumbling:      cfg.WindowSize,
		FlushInterval: cfg.FlushInterval,
	}, producer, pg, log)

	if err := proc.Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("processor stopped", logpkg.F("err", err.Error()))
	}
	log.Info("shutting down")
}
