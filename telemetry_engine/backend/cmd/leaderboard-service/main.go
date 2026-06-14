// Command leaderboard-service is the read model and live fan-out. It consumes
// the Score stream, maintains the ranked board per run in memory (mirrored to
// Redis), serves the REST API, and pushes updates to browsers over WebSocket.
//
// REPLICA NOTE: each instance joins Kafka with a UNIQUE consumer group (group +
// hostname) so every replica sees the full score stream and can serve any
// client from local state. Redis is the durable mirror and run registry.
package main

import (
	"net/http"
	"os"
	"time"

	"github.com/iicpc/track3/telemetry-engine/internal/config"
	"github.com/iicpc/track3/telemetry-engine/internal/leaderboard"
	"github.com/iicpc/track3/telemetry-engine/internal/runtimex"
	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

func main() {
	cfg := config.Load("leaderboard-service")
	log := logpkg.New(cfg.ServiceName)
	ctx, cancel := runtimex.SignalContext()
	defer cancel()

	redis, err := store.NewRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Error("redis required", logpkg.F("err", err.Error()))
		os.Exit(1)
	}
	defer redis.Close()

	pg, err := store.NewPostgres(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Warn("postgres unavailable; contestant history disabled", logpkg.F("err", err.Error()))
	} else {
		defer pg.Close()
	}

	hub := leaderboard.NewHub()
	svc := leaderboard.NewService(hub, redis, log)
	svc.Hydrate(ctx)
	api := leaderboard.NewAPI(svc, hub, pg, log)

	mux := http.NewServeMux()
	api.Routes(mux)

	// Unique group per replica → full-stream fan-out (see package comment).
	host, _ := os.Hostname()
	group := events.GroupLeaderboard + "." + host
	consumer := kafka.NewConsumer(cfg.KafkaBrokers, group, events.TopicScores, cfg.KafkaTLS)
	defer consumer.Close()
	go func() {
		if err := svc.Consume(ctx, consumer); err != nil {
			log.Error("score consumer stopped", logpkg.F("err", err.Error()))
		}
	}()

	// Sandbox metrics consumer: updates the Health field on board entries.
	sbxGroup := events.GroupLeaderboard + ".sandbox." + host
	sbxConsumer := kafka.NewConsumer(cfg.KafkaBrokers, sbxGroup, events.TopicSandboxMetrics, cfg.KafkaTLS)
	defer sbxConsumer.Close()
	go func() {
		log.Info("leaderboard sandbox consumer started")
		if err := svc.ConsumeSandbox(ctx, sbxConsumer); err != nil {
			log.Error("sandbox consumer stopped", logpkg.F("err", err.Error()))
		}
	}()

	apiSrv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	metrics := runtimex.ServeMetrics(cfg.MetricsAddr, log)

	go func() {
		log.Info("leaderboard listening", logpkg.F("addr", cfg.HTTPAddr))
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("api server", logpkg.F("err", err.Error()))
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	runtimex.Shutdown(apiSrv)
	runtimex.Shutdown(metrics)
}
