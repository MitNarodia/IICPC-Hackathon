package config

import "os"

type Config struct {
	CgroupRoot        string
	Runtime           string
	SeccompProfileDir string
	AppArmorProfile   string
	GRPCSocket        string
}

func FromEnv() Config {
	return Config{
		CgroupRoot:        stringEnv("CGROUP_ROOT", "/sys/fs/cgroup"),
		Runtime:           stringEnv("RUNTIME", "runsc"),
		SeccompProfileDir: stringEnv("SECCOMP_PROFILE_DIR", "/profiles/seccomp"),
		AppArmorProfile:   stringEnv("APPARMOR_PROFILE", "track1-sandbox"),
		GRPCSocket:        stringEnv("GRPC_SOCKET", "/run/track1/sandbox-runner.sock"),
	}
}

func stringEnv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
