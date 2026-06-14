package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL         string
	KafkaBrokers        string
	RegistryURL         string
	RegistryRepoPrefix  string
	S3BucketBuildLogs   string
	S3Endpoint          string
	S3AccessKey         string
	S3SecretKey         string
	S3BucketUploads     string
	S3Region            string
	BuildTimeoutSeconds int
	BuildCPULimit       string
	BuildMemLimitMB     int
	K8SBuildNamespace   string
	GRPCPort            string
}

func FromEnv() (Config, error) {
	cfg := Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		KafkaBrokers:        os.Getenv("KAFKA_BROKERS"),
		RegistryURL:         os.Getenv("REGISTRY_URL"),
		RegistryRepoPrefix:  os.Getenv("REGISTRY_REPO_PREFIX"),
		S3BucketBuildLogs:   os.Getenv("S3_BUCKET_BUILD_LOGS"),
		S3Endpoint:          os.Getenv("S3_ENDPOINT"),
		S3AccessKey:         os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:         os.Getenv("S3_SECRET_KEY"),
		S3BucketUploads:     stringEnv("S3_BUCKET_UPLOADS", "raw-uploads"),
		S3Region:            stringEnv("S3_REGION", "us-east-1"),
		BuildTimeoutSeconds: intEnv("BUILD_TIMEOUT_SECONDS", 600),
		BuildCPULimit:       stringEnv("BUILD_CPU_LIMIT", "2"),
		BuildMemLimitMB:     intEnv("BUILD_MEM_LIMIT_MB", 2048),
		K8SBuildNamespace:   stringEnv("K8S_BUILD_NAMESPACE", "track1-build"),
		GRPCPort:            stringEnv("GRPC_PORT", "9090"),
	}
	for name, value := range map[string]string{
		"DATABASE_URL":         cfg.DatabaseURL,
		"KAFKA_BROKERS":        cfg.KafkaBrokers,
		"REGISTRY_URL":         cfg.RegistryURL,
		"REGISTRY_REPO_PREFIX": cfg.RegistryRepoPrefix,
		"S3_BUCKET_BUILD_LOGS": cfg.S3BucketBuildLogs,
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
