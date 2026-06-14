package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/iicpc/track1/submission-engine/pkg/events"
	kafkabus "github.com/iicpc/track1/submission-engine/pkg/events/kafka"
	"github.com/iicpc/track1/submission-engine/pkg/store"
	pgstore "github.com/iicpc/track1/submission-engine/pkg/store/postgres"
	"github.com/iicpc/track1/submission-engine/pkg/store/s3url"
	"github.com/iicpc/track1/submission-engine/services/submission-api/internal/config"
	"github.com/iicpc/track1/submission-engine/services/submission-api/internal/handler"
	apisvc "github.com/iicpc/track1/submission-engine/services/submission-api/internal/service"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	// --- Wire adapters based on env ---
	var repo apisvc.Repository
	var publisher events.Publisher
	var urls store.UploadURLProvider

	// Store: Postgres (real) or in-memory (test).
	if cfg.DatabaseURL != "" {
		pg, err := pgstore.New(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("postgres: %v", err)
		}
		defer pg.Close()
		repo = pg
	} else {
		mem := store.NewMemoryStore()
		repo = mem
	}

	// Bus: Kafka/Redpanda (real) or in-memory.
	if cfg.KafkaBrokers != "" {
		brokers := splitBrokers(cfg.KafkaBrokers)
		producer := kafkabus.NewProducer(brokers)
		defer producer.Close()
		publisher = producer
	} else {
		publisher = events.NewInMemoryBus()
	}

	// Upload URLs: S3/MinIO (real) or static placeholder.
	if cfg.S3Endpoint != "" {
		s3p, err := s3url.New(s3url.Config{
			Endpoint:       cfg.S3Endpoint,
			Region:         envOr("S3_REGION", "us-east-1"),
			AccessKey:      os.Getenv("S3_ACCESS_KEY"),
			SecretKey:      os.Getenv("S3_SECRET_KEY"),
			ForcePathStyle: true,
			BucketUploads:  cfg.S3BucketUploads,
			URLTTLSeconds:  cfg.UploadURLTTLSeconds,
		})
		if err != nil {
			log.Fatalf("s3: %v", err)
		}
		urls = s3p
	} else {
		urls = store.StaticUploadURLProvider{BaseURL: "s3://" + cfg.S3BucketUploads}
	}

	h := handler.Handler{Service: apisvc.Service{Repo: repo, UploadURLs: urls, Publisher: publisher}}
	mux := http.NewServeMux()
	h.Register(mux)
	// mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })

	srv := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	log.Printf("submission-api listening on :%s", cfg.HTTPPort)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func splitBrokers(s string) []string {
	return strings.Split(s, ",")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
