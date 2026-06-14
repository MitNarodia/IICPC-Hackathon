package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/iicpc/track1/submission-engine/pkg/bootstrap"
	"github.com/iicpc/track1/submission-engine/pkg/events"
	kafkabus "github.com/iicpc/track1/submission-engine/pkg/events/kafka"
	dockerorch "github.com/iicpc/track1/submission-engine/pkg/orchestrator/docker"
	"github.com/iicpc/track1/submission-engine/services/sandbox-runner/internal/config"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := config.FromEnv()

	brokers := strings.Split(envOr("KAFKA_BROKERS", "localhost:9092"), ",")
	producer := kafkabus.NewProducer(brokers)
	defer producer.Close()

	// NEW: Wire docker orchestrator
	orch, err := bootstrap.WireOrchestrator(ctx)
	if err != nil {
		log.Fatalf("orchestrator: %v", err)
	}
	defer orch.(*dockerorch.Orchestrator).Close()

	consumer := kafkabus.NewConsumer(kafkabus.ConsumerConfig{
		Brokers: brokers,
		GroupID: "sandbox-runner",
		Topics:  []string{string(events.TeardownRequested)},
	})
	defer consumer.Close()

	consumer.Handle(events.TeardownRequested, func(ctx context.Context, env events.Envelope) error {
		log.Printf("sandbox-runner: teardown signal for %s — stopping container", env.SubmissionID)
		
		// NEW: Actual teardown logic
		if err := orch.StopSandbox(ctx, env.SubmissionID); err != nil {
			log.Printf("sandbox-runner: StopSandbox error for %s: %v", env.SubmissionID, err)
			return err // Return err to potentially retry
		}

		return nil
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	srv := &http.Server{Addr: ":9090", Handler: mux}
	go func() { _ = srv.ListenAndServe() }()

	log.Printf("sandbox-runner started: runtime=%s cgroup_root=%s socket=%s", cfg.Runtime, cfg.CgroupRoot, cfg.GRPCSocket)
	if err := consumer.Run(ctx); err != nil {
		log.Printf("sandbox-runner: consumer exited: %v", err)
	}
	srv.Shutdown(context.Background())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
