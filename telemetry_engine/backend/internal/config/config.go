// Package config loads service configuration from environment variables
// (12-factor). Every service constructs a Config at boot; there are no config
// files baked into the image. Defaults are dev-friendly so `docker compose up`
// works with zero env wiring, while production overrides everything via env.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the union of every setting any Track 3 service needs. A given
// service only reads the fields relevant to it; unused fields are harmless.
type Config struct {
	// Identity
	ServiceName string

	// Redpanda / Kafka
	KafkaBrokers []string
	KafkaTLS     bool

	// PostgreSQL + TimescaleDB
	PostgresDSN string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// HTTP listeners
	HTTPAddr    string // public API / ingestion HTTP
	MetricsAddr string // Prometheus /metrics scrape target

	// Stream processing knobs
	WindowSize       time.Duration // tumbling window width
	SlideInterval    time.Duration // sliding window advance
	RollingRetention time.Duration // how much history rolling stats keep
	FlushInterval    time.Duration // how often the processor emits a window

	// Scoring weights (sum need not equal 1; normalized at use)
	WeightLatency     float64
	WeightThroughput  float64
	WeightCorrectness float64
	WeightStability   float64
}

// Load reads the environment and fills a Config with sane defaults.
func Load(serviceName string) Config {
	return Config{
		ServiceName:  serviceName,
		KafkaBrokers: splitCSV(env("KAFKA_BROKERS", "localhost:9092")),
		KafkaTLS:     envBool("KAFKA_TLS", false),
		PostgresDSN: env("POSTGRES_DSN",
			"postgres://track3:track3@localhost:5432/track3?sslmode=disable"),
		RedisAddr:         env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     env("REDIS_PASSWORD", ""),
		RedisDB:           envInt("REDIS_DB", 0),
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		MetricsAddr:       env("METRICS_ADDR", ":9100"),
		WindowSize:        envDur("WINDOW_SIZE", 5*time.Second),
		SlideInterval:     envDur("SLIDE_INTERVAL", 1*time.Second),
		RollingRetention:  envDur("ROLLING_RETENTION", 60*time.Second),
		FlushInterval:     envDur("FLUSH_INTERVAL", 1*time.Second),
		WeightLatency:     envFloat("WEIGHT_LATENCY", 0.35),
		WeightThroughput:  envFloat("WEIGHT_THROUGHPUT", 0.30),
		WeightCorrectness: envFloat("WEIGHT_CORRECTNESS", 0.25),
		WeightStability:   envFloat("WEIGHT_STABILITY", 0.10),
	}
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, ok := os.LookupEnv(k); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v, ok := os.LookupEnv(k); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envBool(k string, def bool) bool {
	if v, ok := os.LookupEnv(k); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envDur(k string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(k); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
