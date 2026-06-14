package service

import (
	"strings"
	"testing"
)

func TestBuildRunSpecHardeningArgs(t *testing.T) {
	spec, err := Runner{Runtime: "runsc", SeccompProfileDir: "/profiles/seccomp", AppArmorProfile: "track1-sandbox"}.BuildRunSpec(RunRequest{
		Image:       "registry/submission@sha256:abc",
		Entrypoint:  []string{"/app/bot"},
		CPUCores:    1,
		CPUSet:      "5",
		MemoryBytes: 512 * 1024 * 1024,
		PidsMax:     256,
		Port:        8080,
	})
	if err != nil {
		t.Fatalf("BuildRunSpec() error = %v", err)
	}
	joined := strings.Join(spec.Args, " ")
	for _, want := range []string{"--rootless", "--no-new-privileges", "--read-only", "--cap-drop=ALL", "--cpuset-cpus=5", "--apparmor=track1-sandbox"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %s: %s", want, joined)
		}
	}
}

func TestBuildRunSpecRejectsLatest(t *testing.T) {
	_, err := Runner{Runtime: "runsc"}.BuildRunSpec(RunRequest{Image: "registry/submission:latest", Port: 8080})
	if err == nil {
		t.Fatal("expected image digest pinning error")
	}
}
