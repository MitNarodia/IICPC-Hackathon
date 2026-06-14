package service

import (
	"errors"
	"fmt"
	"strings"
)

type RunRequest struct {
	Image       string
	Entrypoint  []string
	CPUCores    int
	CPUSet      string
	MemoryBytes int64
	PidsMax     int
	Port        int
}

type RunSpec struct {
	Runtime string
	Args    []string
	Limits  Limits
}

type Limits struct {
	CPUCores    int
	CPUSet      string
	MemoryBytes int64
	PidsMax     int
}

type Stats struct {
	CPUPct   float64
	MemMB    float64
	Restarts int
}

type Runner struct {
	Runtime           string
	SeccompProfileDir string
	AppArmorProfile   string
}

func (r Runner) BuildRunSpec(req RunRequest) (RunSpec, error) {
	runtime := r.Runtime
	if runtime == "" {
		runtime = "runsc"
	}
	if runtime != "runsc" && runtime != "crun" {
		return RunSpec{}, fmt.Errorf("unsupported runtime %q", runtime)
	}
	if strings.Contains(req.Image, ":latest") || !strings.Contains(req.Image, "@sha256:") {
		return RunSpec{}, errors.New("image must be pinned by digest")
	}
	if req.CPUCores <= 0 {
		req.CPUCores = 1
	}
	if req.MemoryBytes <= 0 {
		req.MemoryBytes = 512 * 1024 * 1024
	}
	if req.PidsMax <= 0 {
		req.PidsMax = 256
	}
	if req.Port < 1024 || req.Port > 65535 {
		return RunSpec{}, errors.New("port must be in [1024,65535]")
	}
	args := []string{
		"run",
		"--rootless",
		"--no-new-privileges",
		"--read-only",
		"--cap-drop=ALL",
		fmt.Sprintf("--memory=%d", req.MemoryBytes),
		fmt.Sprintf("--pids-limit=%d", req.PidsMax),
		fmt.Sprintf("--cpus=%d", req.CPUCores),
	}
	if req.CPUSet != "" {
		args = append(args, "--cpuset-cpus="+req.CPUSet)
	}
	if r.SeccompProfileDir != "" {
		args = append(args, "--seccomp-profile="+strings.TrimRight(r.SeccompProfileDir, "/")+"/track1-sandbox.json")
	}
	if r.AppArmorProfile != "" {
		args = append(args, "--apparmor="+r.AppArmorProfile)
	}
	args = append(args, req.Image)
	args = append(args, req.Entrypoint...)
	return RunSpec{
		Runtime: runtime,
		Args:    args,
		Limits:  Limits{CPUCores: req.CPUCores, CPUSet: req.CPUSet, MemoryBytes: req.MemoryBytes, PidsMax: req.PidsMax},
	}, nil
}
