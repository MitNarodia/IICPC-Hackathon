// Package bootstrap wires real infrastructure adapters based on environment
// configuration. Each service main.go calls bootstrap.Wire() to get its deps.
package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/iicpc/track1/submission-engine/pkg/events"
	kafkabus "github.com/iicpc/track1/submission-engine/pkg/events/kafka"
	"github.com/iicpc/track1/submission-engine/pkg/orchestrator"
	dockerorch "github.com/iicpc/track1/submission-engine/pkg/orchestrator/docker"
	pgstore "github.com/iicpc/track1/submission-engine/pkg/store/postgres"
	"github.com/iicpc/track1/submission-engine/pkg/store/redisreg"
	"github.com/iicpc/track1/submission-engine/pkg/store/s3url"
)

// Deps holds all infrastructure dependencies a service might need.
type Deps struct {
	Store        *pgstore.Store         // nil if not needed
	Registry     *redisreg.Registry     // nil if not needed
	S3           *s3url.Provider        // nil if not needed
	Producer     events.Publisher       // always wired (kafka or in-memory)
	Consumer     *kafkabus.Consumer     // nil if the service doesn't consume
	Orchestrator orchestrator.Orchestrator // nil if the service doesn't orchestrate
}

// Close releases all held resources.
func (d *Deps) Close() {
	if d.Store != nil {
		d.Store.Close()
	}
	if d.Registry != nil {
		_ = d.Registry.Close()
	}
	if p, ok := d.Producer.(*kafkabus.Producer); ok {
		_ = p.Close()
	}
	if d.Consumer != nil {
		_ = d.Consumer.Close()
	}
}

// WireStore connects to Postgres.
func WireStore(ctx context.Context) (*pgstore.Store, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	return pgstore.New(ctx, dsn)
}

// WireRegistry connects to Redis.
func WireRegistry(ctx context.Context) (*redisreg.Registry, error) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}
	return redisreg.New(ctx, url)
}

// WireS3 builds the S3/MinIO provider.
func WireS3() (*s3url.Provider, error) {
	cfg := s3url.Config{
		Endpoint:       os.Getenv("S3_ENDPOINT"),
		Region:         envOr("S3_REGION", "us-east-1"),
		AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		SecretKey:      os.Getenv("S3_SECRET_KEY"),
		ForcePathStyle: envOr("S3_FORCE_PATH_STYLE", "true") == "true",
		BucketUploads:  envOr("S3_BUCKET_UPLOADS", "raw-uploads"),
		BucketLogs:     envOr("S3_BUCKET_BUILD_LOGS", "build-logs"),
		URLTTLSeconds:  envInt("UPLOAD_URL_TTL_SECONDS", 900),
	}
	return s3url.New(cfg)
}

// WireProducer creates a Kafka producer.
func WireProducer() (*kafkabus.Producer, error) {
	brokers := brokerList()
	if len(brokers) == 0 {
		return nil, fmt.Errorf("KAFKA_BROKERS is required")
	}
	return kafkabus.NewProducer(brokers), nil
}

// WireConsumer creates a Kafka consumer for the given group and topics.
func WireConsumer(groupID string, topics []string) (*kafkabus.Consumer, error) {
	brokers := brokerList()
	if len(brokers) == 0 {
		return nil, fmt.Errorf("KAFKA_BROKERS is required")
	}
	return kafkabus.NewConsumer(kafkabus.ConsumerConfig{
		Brokers: brokers,
		GroupID: groupID,
		Topics:  topics,
	}), nil
}

// WireOrchestrator creates a Docker orchestrator.
func WireOrchestrator(ctx context.Context) (orchestrator.Orchestrator, error) {
	network := envOr("SANDBOX_NETWORK", "track1-sandbox-net")
	timeout := time.Duration(envInt("BUILD_TIMEOUT_SECONDS", 600)) * time.Second
	cpus := envInt("SANDBOX_CPU", 1)
	memMB := envInt("SANDBOX_MEMORY_MB", 512)
	dockerHost := os.Getenv("DOCKER_HOST")

	orch, err := dockerorch.New(ctx, dockerorch.Config{
		DockerHost:     dockerHost,
		SandboxNetwork: network,
		BuildTimeout:   timeout,
		CPUCores:       cpus,
		MemoryMB:       memMB,
	})
	if err != nil {
		return nil, fmt.Errorf("orchestrator wiring: %w", err)
	}
	return orch, nil
}

// EnsureTopics creates all Track 1 topics with the given partition count.
func EnsureTopics(ctx context.Context, partitions int) error {
	brokers := brokerList()
	topics := []string{
		"submission.created", "submission.uploaded",
		"validation.failed", "build.requested", "build.succeeded", "build.failed",
		"scan.passed", "scan.failed",
		"deployment.requested", "deployment.ready", "deployment.failed",
		"health.ready", "health.degraded", "health.recovered",
		"teardown.requested", "teardown.completed",
		"dead-letter",
	}
	return kafkabus.EnsureTopics(ctx, brokers, topics, partitions)
}

// MustWire is a convenience that logs.Fatal on any error.
func MustWire(name string, err error) {
	if err != nil {
		log.Fatalf("[%s] bootstrap failed: %v", name, err)
	}
}

func brokerList() []string {
	raw := os.Getenv("KAFKA_BROKERS")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n := 0
	fmt.Sscanf(v, "%d", &n)
	if n == 0 {
		return fallback
	}
	return n
}
