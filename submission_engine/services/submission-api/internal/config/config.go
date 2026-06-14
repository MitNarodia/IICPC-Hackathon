package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL         string
	KafkaBrokers        string
	RedisURL            string
	S3Endpoint          string
	S3BucketUploads     string
	S3Region            string
	JWTPublicKey        string
	UploadURLTTLSeconds int
	MaxUploadBytes      int64
	RateLimitPerMin     int
	HTTPPort            string
	LogLevel            string
}

func FromEnv() (Config, error) {
	cfg := Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		KafkaBrokers:        os.Getenv("KAFKA_BROKERS"),
		RedisURL:            os.Getenv("REDIS_URL"),
		S3Endpoint:          os.Getenv("S3_ENDPOINT"),
		S3BucketUploads:     os.Getenv("S3_BUCKET_UPLOADS"),
		S3Region:            os.Getenv("S3_REGION"),
		JWTPublicKey:        os.Getenv("JWT_PUBLIC_KEY"),
		UploadURLTTLSeconds: intEnv("UPLOAD_URL_TTL_SECONDS", 900),
		MaxUploadBytes:      int64(intEnv("MAX_UPLOAD_BYTES", 104857600)),
		RateLimitPerMin:     intEnv("RATE_LIMIT_PER_MIN", 10),
		HTTPPort:            stringEnv("HTTP_PORT", "8080"),
		LogLevel:            stringEnv("LOG_LEVEL", "info"),
	}
	for name, value := range map[string]string{
		"DATABASE_URL":      cfg.DatabaseURL,
		"KAFKA_BROKERS":     cfg.KafkaBrokers,
		"S3_ENDPOINT":       cfg.S3Endpoint,
		"S3_BUCKET_UPLOADS": cfg.S3BucketUploads,
		"S3_REGION":         cfg.S3Region,
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
