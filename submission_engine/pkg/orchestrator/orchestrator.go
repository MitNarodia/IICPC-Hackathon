// Package orchestrator defines the abstraction for running builds and sandboxes.
package orchestrator

import (
	"context"
	"io"

	"github.com/iicpc/track1/submission-engine/pkg/security"
)

// BuildOutcome is returned after a successful build.
type BuildOutcome struct {
	ImageRef    string // e.g. "track1/submission:sub-id"
	ImageDigest string // "sha256:..."
	LogsURI     string // where build logs were stored
}

// EndpointInfo describes how to reach a running sandbox.
type EndpointInfo struct {
	Address           string // host:port or container-name:port
	Port              int
	ContainerOrPodName string
}

// Orchestrator drives build and sandbox lifecycle.
type Orchestrator interface {
	// RunBuild compiles a submission's source/binary into a runnable image.
	RunBuild(ctx context.Context, submissionID string, dockerfile string, contextTar io.Reader) (BuildOutcome, error)

	// StartSandbox launches the built image under hardening controls.
	StartSandbox(ctx context.Context, submissionID string, imageRef string, declaredPort int, spec security.SandboxPodRequest) (EndpointInfo, error)

	// StopSandbox tears down everything for a submission (idempotent).
	StopSandbox(ctx context.Context, submissionID string) error
}
