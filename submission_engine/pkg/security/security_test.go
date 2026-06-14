package security

import (
	"strings"
	"testing"
)

func TestBuildSandboxPodHardening(t *testing.T) {
	pod, err := BuildSandboxPod(SandboxPodRequest{
		SubmissionID: "018fd6c2-5a6b-7abc-8def-111111111111",
		ImageRef:     "registry/submission@sha256:abc",
		Entrypoint:   "/app/bot",
		DeclaredPort: 8080,
		CPUCores:     1,
		MemoryMB:     512,
	})
	if err != nil {
		t.Fatalf("BuildSandboxPod() error = %v", err)
	}
	if err := ValidateSandboxPod(pod); err != nil {
		t.Fatalf("ValidateSandboxPod() error = %v", err)
	}
	if pod.Spec.Containers[0].Resources.Limits["cpu"] != "1" {
		t.Fatalf("cpu limit = %s", pod.Spec.Containers[0].Resources.Limits["cpu"])
	}
	if !pod.Spec.Containers[0].SecurityContext.ReadOnlyRootFilesystem {
		t.Fatal("readOnlyRootFilesystem must be true")
	}
}

func TestBuildSandboxPodRejectsUnsafeImage(t *testing.T) {
	_, err := BuildSandboxPod(SandboxPodRequest{
		SubmissionID: "018fd6c2-5a6b-7abc-8def-111111111111",
		ImageRef:     "registry/submission:latest",
		DeclaredPort: 8080,
	})
	if err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("expected pinned digest error, got %v", err)
	}
}

func TestNetworkPoliciesDefaultDenyEgress(t *testing.T) {
	policies, err := BuildSandboxNetworkPolicies(SandboxPodRequest{
		SubmissionID: "018fd6c2-5a6b-7abc-8def-111111111111",
		ImageRef:     "registry/submission@sha256:abc",
		DeclaredPort: 8080,
	})
	if err != nil {
		t.Fatalf("BuildSandboxNetworkPolicies() error = %v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("len(policies) = %d, want 2", len(policies))
	}
	if len(policies[0].Spec.Egress) != 0 || len(policies[0].Spec.Ingress) != 0 {
		t.Fatal("default-deny policy must contain no ingress or egress rules")
	}
}

func TestSeccompBlocksDangerousSyscalls(t *testing.T) {
	profile := DefaultSeccomp()
	allowed := map[string]bool{}
	for _, group := range profile.Syscalls {
		for _, name := range group.Names {
			allowed[name] = true
		}
	}
	for _, syscall := range DangerousSyscalls() {
		if allowed[syscall] {
			t.Fatalf("dangerous syscall %s must not be allowed", syscall)
		}
	}
}
