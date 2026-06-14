package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL          string
	KafkaBrokers         string
	S3Endpoint           string
	S3BucketUploads      string
	MaxUploadBytes       int64
	MaxDecompressedBytes int64
	MaxFiles             int
	GRPCPort             string
}

func FromEnv() (Config, error) {
	cfg := Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		KafkaBrokers:         os.Getenv("KAFKA_BROKERS"),
		S3Endpoint:           os.Getenv("S3_ENDPOINT"),
		S3BucketUploads:      os.Getenv("S3_BUCKET_UPLOADS"),
		MaxUploadBytes:       int64(intEnv("MAX_UPLOAD_BYTES", 104857600)),
		MaxDecompressedBytes: int64(intEnv("MAX_DECOMPRESSED_BYTES", 536870912)),
		MaxFiles:             intEnv("MAX_FILES", 10000),
		GRPCPort:             stringEnv("GRPC_PORT", "9090"),
	}
	for name, value := range map[string]string{
		"DATABASE_URL":      cfg.DatabaseURL,
		"KAFKA_BROKERS":     cfg.KafkaBrokers,
		"S3_ENDPOINT":       cfg.S3Endpoint,
		"S3_BUCKET_UPLOADS": cfg.S3BucketUploads,
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
