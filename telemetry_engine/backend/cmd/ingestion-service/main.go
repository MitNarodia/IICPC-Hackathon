// Command ingestion-service is Track 3's front door. It accepts telemetry from
// Track 2 (bot fleet) and Track 1 (sandbox) over HTTP and WebSocket, validates
// it, and produces it to Redpanda for the downstream analytics pipeline.
package main

import (
	"net/http"
	"time"

	"github.com/iicpc/track3/telemetry-engine/internal/config"
	"github.com/iicpc/track3/telemetry-engine/internal/ingestion"
	"github.com/iicpc/track3/telemetry-engine/internal/runtimex"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

func main() {
	cfg := config.Load("ingestion-service")
	log := logpkg.New(cfg.ServiceName)
	ctx, cancel := runtimex.SignalContext()
	defer cancel()

	if err := kafka.EnsureTopics(ctx, cfg.KafkaBrokers, 12, 1); err != nil {
		log.Warn("ensure topics", logpkg.F("err", err.Error()))
	}
	producer := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTLS)
	defer producer.Close()

	server := ingestion.NewServer(producer, log)
	mux := http.NewServeMux()
	server.Routes(mux)

	api := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	metrics := runtimex.ServeMetrics(cfg.MetricsAddr, log)

	go func() {
		log.Info("ingestion listening", logpkg.F("addr", cfg.HTTPAddr))
		if err := api.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("api server", logpkg.F("err", err.Error()))
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	runtimex.Shutdown(api)
	runtimex.Shutdown(metrics)
}
