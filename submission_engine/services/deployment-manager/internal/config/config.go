package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL              string
	KafkaBrokers             string
	RedisURL                 string
	K8SSandboxNamespace      string
	RuntimeClass             string
	DefaultCPUCores          int
	DefaultMemMB             int
	SandboxNodeSelector      map[string]string
	SandboxTolerationKey     string
	SandboxTolerationValue   string
	ReconcileIntervalSeconds int
	LeaderElection           bool
	GRPCPort                 string
}

func FromEnv() (Config, error) {
	key, value := parsePair(stringEnv("SANDBOX_TOLERATION", "workload=untrusted"))
	cfg := Config{
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		KafkaBrokers:             os.Getenv("KAFKA_BROKERS"),
		RedisURL:                 os.Getenv("REDIS_URL"),
		K8SSandboxNamespace:      stringEnv("K8S_SANDBOX_NAMESPACE", "track1-sandbox"),
		RuntimeClass:             stringEnv("RUNTIME_CLASS", "gvisor"),
		DefaultCPUCores:          intEnv("DEFAULT_CPU_CORES", 1),
		DefaultMemMB:             intEnv("DEFAULT_MEM_MB", 512),
		SandboxNodeSelector:      pairMap(stringEnv("SANDBOX_NODE_SELECTOR", "sandbox=true")),
		SandboxTolerationKey:     key,
		SandboxTolerationValue:   value,
		ReconcileIntervalSeconds: intEnv("RECONCILE_INTERVAL_SECONDS", 15),
		LeaderElection:           boolEnv("LEADER_ELECTION", true),
		GRPCPort:                 stringEnv("GRPC_PORT", "9090"),
	}
	for name, value := range map[string]string{
		"DATABASE_URL":  cfg.DatabaseURL,
		"KAFKA_BROKERS": cfg.KafkaBrokers,
		"REDIS_URL":     cfg.RedisURL,
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

func boolEnv(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value == "true" || value == "1"
}

func stringEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func pairMap(value string) map[string]string {
	key, val := parsePair(value)
	return map[string]string{key: val}
}

func parsePair(value string) (string, string) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return value, ""
	}
	return parts[0], parts[1]
}
