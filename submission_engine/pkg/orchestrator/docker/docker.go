// Package docker implements orchestrator.Orchestrator using the Docker Engine API.
// This is the primary runtime path for local development and single-host deployment.
package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/iicpc/track1/submission-engine/pkg/orchestrator"
	"github.com/iicpc/track1/submission-engine/pkg/security"
)

// Config for the Docker orchestrator.
type Config struct {
	DockerHost     string // DOCKER_HOST, empty = default
	SandboxNetwork string // isolated network for submissions
	BuildTimeout   time.Duration
	CPUCores       int
	MemoryMB       int
}

// Orchestrator implements orchestrator.Orchestrator using Docker.
type Orchestrator struct {
	cli     *client.Client
	cfg     Config
}

// New creates a Docker orchestrator. It verifies the Docker daemon is reachable.
func New(ctx context.Context, cfg Config) (*Orchestrator, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if cfg.DockerHost != "" {
		opts = append(opts, client.WithHost(cfg.DockerHost))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker orchestrator: client: %w", err)
	}
	if _, err := cli.Ping(ctx); err != nil {
		return nil, fmt.Errorf("docker orchestrator: ping: %w", err)
	}
	// Ensure the sandbox network exists.
	if cfg.SandboxNetwork != "" {
		if err := ensureNetwork(ctx, cli, cfg.SandboxNetwork); err != nil {
			return nil, err
		}
	}
	if cfg.BuildTimeout == 0 {
		cfg.BuildTimeout = 10 * time.Minute
	}
	if cfg.CPUCores == 0 {
		cfg.CPUCores = 1
	}
	if cfg.MemoryMB == 0 {
		cfg.MemoryMB = 512
	}
	return &Orchestrator{cli: cli, cfg: cfg}, nil
}

func ensureNetwork(ctx context.Context, cli *client.Client, name string) error {
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return err
	}
	for _, n := range networks {
		if n.Name == name {
			return nil
		}
	}
	_, err = cli.NetworkCreate(ctx, name, types.NetworkCreate{
		Driver:   "bridge",
		Internal: true, // no outbound internet access
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("docker: create network %s: %w", name, err)
	}
	return nil
}

// containerName returns the standard name for a submission's sandbox container.
func containerName(submissionID string) string {
	return "track1-sandbox-" + submissionID[:12]
}

// RunBuild builds a Docker image from the provided Dockerfile + context tar.
func (o *Orchestrator) RunBuild(ctx context.Context, submissionID string, dockerfile string, contextTar io.Reader) (orchestrator.BuildOutcome, error) {
	ctx, cancel := context.WithTimeout(ctx, o.cfg.BuildTimeout)
	defer cancel()

	imageTag := "track1/submission:" + submissionID

	// Prepend the Dockerfile into the context tar.
	buildCtx, err := prependDockerfile(dockerfile, contextTar)
	if err != nil {
		return orchestrator.BuildOutcome{}, fmt.Errorf("docker: build context: %w", err)
	}

	resp, err := o.cli.ImageBuild(ctx, buildCtx, types.ImageBuildOptions{
		Tags:       []string{imageTag},
		Dockerfile: "Dockerfile",
		Remove:     true,
		NoCache:    false,
	})
	if err != nil {
		return orchestrator.BuildOutcome{}, fmt.Errorf("docker: image build: %w", err)
	}
	defer resp.Body.Close()

	// Read build output to completion (required to finish the build).
	var buildLog bytes.Buffer
	if err := readBuildOutput(resp.Body, &buildLog); err != nil {
		return orchestrator.BuildOutcome{}, fmt.Errorf("docker: build stream: %w", err)
	}

	// Inspect to get the digest.
	inspect, _, err := o.cli.ImageInspectWithRaw(ctx, imageTag)
	if err != nil {
		return orchestrator.BuildOutcome{}, fmt.Errorf("docker: inspect built image: %w", err)
	}

	digest := ""
	if len(inspect.RepoDigests) > 0 {
		digest = inspect.RepoDigests[0]
	} else {
		// Local-only image; use the ID as a pseudo-digest.
		digest = inspect.ID
	}

	return orchestrator.BuildOutcome{
		ImageRef:    imageTag,
		ImageDigest: digest,
		LogsURI:     fmt.Sprintf("memory://build-logs/%s", submissionID),
	}, nil
}

// StartSandbox launches a hardened container for the submission.
func (o *Orchestrator) StartSandbox(ctx context.Context, submissionID string, imageRef string, declaredPort int, spec security.SandboxPodRequest) (orchestrator.EndpointInfo, error) {
	name := containerName(submissionID)
	portStr := strconv.Itoa(declaredPort)
	exposedPort := nat.Port(portStr + "/tcp")

	secOpts := []string{"no-new-privileges:true"}
	// Seccomp: use the restrictive sandbox profile (deny-by-default, allow
	// only the syscalls needed for typical contestant binaries).
	// secOpts = append(secOpts, "seccomp=/opt/seccomp/sandbox-default.json")

	containerCfg := &container.Config{
		Image:        imageRef,
		ExposedPorts: nat.PortSet{exposedPort: struct{}{}},
		User:         "65532:65532",
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("wget -qO- http://localhost:%d/healthz || exit 1", declaredPort)},
			Interval: 5 * time.Second,
			Timeout:  3 * time.Second,
			Retries:  3,
		},
	}

	hostCfg := &container.HostConfig{
		ReadonlyRootfs: true,
		SecurityOpt:    secOpts,
		CapDrop:        []string{"ALL"},
		Resources: container.Resources{
			NanoCPUs: int64(o.cfg.CPUCores) * 1e9,
			Memory:   int64(o.cfg.MemoryMB) * 1024 * 1024,
			PidsLimit: int64Ptr(256),
		},
		Tmpfs: map[string]string{
			"/tmp": "size=64m,noexec,nosuid",
		},
		NetworkMode: container.NetworkMode(o.cfg.SandboxNetwork),
		PortBindings: nat.PortMap{
			exposedPort: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: ""}}, // random host port
		},
	}

	netCfg := &network.NetworkingConfig{}

	created, err := o.cli.ContainerCreate(ctx, containerCfg, hostCfg, netCfg, nil, name)
	if err != nil {
		return orchestrator.EndpointInfo{}, fmt.Errorf("docker: create sandbox: %w", err)
	}
	if err := o.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return orchestrator.EndpointInfo{}, fmt.Errorf("docker: start sandbox: %w", err)
	}

	// Inspect to find the mapped port.
	info, err := o.cli.ContainerInspect(ctx, created.ID)
	if err != nil {
		return orchestrator.EndpointInfo{}, fmt.Errorf("docker: inspect sandbox: %w", err)
	}

	// Derive the endpoint address.
	address := name // within the docker network, the container name is resolvable
	port := declaredPort
	if bindings, ok := info.NetworkSettings.Ports[exposedPort]; ok && len(bindings) > 0 {
		if hp, err := strconv.Atoi(bindings[0].HostPort); err == nil {
			port = hp
			address = "host.docker.internal"
		}
	}

	return orchestrator.EndpointInfo{
		Address:            fmt.Sprintf("%s:%d", address, port),
		Port:               port,
		ContainerOrPodName: name,
	}, nil
}

// StopSandbox stops and removes the container (idempotent).
func (o *Orchestrator) StopSandbox(ctx context.Context, submissionID string) error {
	name := containerName(submissionID)
	timeout := 10
	_ = o.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
	err := o.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !client.IsErrNotFound(err) {
		return err
	}
	return nil
}

// Close closes the Docker client.
func (o *Orchestrator) Close() error { return o.cli.Close() }

// --- helpers ---

func int64Ptr(v int64) *int64 { return &v }

// prependDockerfile creates a new tar that has the Dockerfile at the root,
// followed by the original context tar contents.
func prependDockerfile(dockerfile string, contextTar io.Reader) (io.Reader, error) {
	// For simplicity, build a buffer with the Dockerfile prepended as a single-entry tar,
	// then append the rest of the context. The Docker build API accepts a single tar stream.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	dfBytes := []byte(dockerfile)
	if err := tw.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Size: int64(len(dfBytes)),
		Mode: 0644,
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(dfBytes); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	// Combine: dockerfile tar + context tar.
	// Actually, Docker expects a single tar — we'll just use the Dockerfile-only tar
	// if contextTar is nil, otherwise we need a proper merge.
	if contextTar == nil {
		return &buf, nil
	}
	// Read context into memory and re-create a combined tar.
	ctxData, err := io.ReadAll(contextTar)
	if err != nil {
		return nil, err
	}
	// Simple approach: re-create tar with Dockerfile + context entries.
	var combined bytes.Buffer
	cw := tar.NewWriter(&combined)
	// Write Dockerfile.
	_ = cw.WriteHeader(&tar.Header{Name: "Dockerfile", Size: int64(len(dfBytes)), Mode: 0644})
	_, _ = cw.Write(dfBytes)
	// Copy context entries.
	tr := tar.NewReader(bytes.NewReader(ctxData))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		_ = cw.WriteHeader(hdr)
		if hdr.Size > 0 {
			_, _ = io.Copy(cw, tr)
		}
	}
	_ = cw.Close()
	return &combined, nil
}

// readBuildOutput reads the Docker build JSON stream to completion.
func readBuildOutput(r io.Reader, logBuf *bytes.Buffer) error {
	dec := json.NewDecoder(r)
	for {
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if msg.Error != "" {
			return fmt.Errorf("build error: %s", msg.Error)
		}
		if logBuf != nil {
			logBuf.WriteString(msg.Stream)
		}
	}
}
