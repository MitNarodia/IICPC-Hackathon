package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL          string
	KafkaBrokers         string
	ProbeIntervalSeconds int
	UnhealthyThreshold   int
	HealthyThreshold     int
	ProbeTimeoutMS       int
	LatencySLOMS         float64
	HTTPPort             string
}

func FromEnv() (Config, error) {
	cfg := Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		KafkaBrokers:         os.Getenv("KAFKA_BROKERS"),
		ProbeIntervalSeconds: intEnv("PROBE_INTERVAL_SECONDS", 5),
		UnhealthyThreshold:   intEnv("UNHEALTHY_THRESHOLD", 3),
		HealthyThreshold:     intEnv("HEALTHY_THRESHOLD", 2),
		ProbeTimeoutMS:       intEnv("PROBE_TIMEOUT_MS", 2000),
		LatencySLOMS:         float64(intEnv("LATENCY_SLO_MS", 500)),
		HTTPPort:             stringEnv("HTTP_PORT", "8080"),
	}
	for name, value := range map[string]string{
		"DATABASE_URL":  cfg.DatabaseURL,
		"KAFKA_BROKERS": cfg.KafkaBrokers,
	} {
		if value == "" {
			return Config{}, fmt.Errorf("%s is required", name)
		}
	}
	return cfg, nil
}

func intEnv(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func stringEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
