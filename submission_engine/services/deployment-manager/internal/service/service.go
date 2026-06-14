package service

import (
	"context"
	"fmt"
	"time"

	"github.com/iicpc/track1/submission-engine/pkg/events"
	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/security"
	"github.com/iicpc/track1/submission-engine/pkg/store"
)

type Repository interface {
	store.DeploymentRepository
	store.Registry
}

type Manager struct {
	Repo          Repository
	Publisher     events.Publisher
	Namespace     string
	RuntimeClass  string
	NodeSelector  map[string]string
	TolerationKey string
	TolerationVal string
	DefaultCPU    int
	DefaultMemory int
}

type DeployRequest struct {
	SubmissionID string
	ImageRef     string
	Entrypoint   string
	DeclaredPort int
	CPUCores     int
	MemoryMB     int
}

type DeploymentPlan struct {
	Manifests  security.SandboxManifests
	Deployment models.Deployment
	Endpoint   models.Endpoint
}

func (m Manager) Plan(req DeployRequest) (DeploymentPlan, error) {
	cpu := req.CPUCores
	if cpu == 0 {
		cpu = defaultInt(m.DefaultCPU, 1)
	}
	mem := req.MemoryMB
	if mem == 0 {
		mem = defaultInt(m.DefaultMemory, 512)
	}
	namespace := defaultString(m.Namespace, security.DefaultSandboxNamespace)
	manifests, err := security.BuildSandboxManifests(security.SandboxPodRequest{
		SubmissionID:  req.SubmissionID,
		ImageRef:      req.ImageRef,
		Entrypoint:    req.Entrypoint,
		DeclaredPort:  req.DeclaredPort,
		CPUCores:      cpu,
		MemoryMB:      mem,
		Namespace:     namespace,
		RuntimeClass:  defaultString(m.RuntimeClass, security.DefaultRuntimeClass),
		NodeSelector:  m.NodeSelector,
		TolerationKey: defaultString(m.TolerationKey, "workload"),
		TolerationVal: defaultString(m.TolerationVal, "untrusted"),
	})
	if err != nil {
		return DeploymentPlan{}, err
	}
	deploymentID, err := models.NewUUIDv7()
	if err != nil {
		return DeploymentPlan{}, err
	}
	endpointID, err := models.NewUUIDv7()
	if err != nil {
		return DeploymentPlan{}, err
	}
	serviceName := security.SandboxName(req.SubmissionID)
	deployment := models.Deployment{
		ID:           deploymentID,
		SubmissionID: req.SubmissionID,
		Status:       models.DeploymentScheduling,
		PodName:      manifests.Pod.Metadata.Name,
		Namespace:    namespace,
		CPUCores:     cpu,
		MemoryMB:     mem,
		RuntimeClass: defaultString(m.RuntimeClass, security.DefaultRuntimeClass),
		CreatedAt:    time.Now().UTC(),
	}
	endpoint := models.Endpoint{
		ID:           endpointID,
		SubmissionID: req.SubmissionID,
		DeploymentID: deploymentID,
		InternalURL:  fmt.Sprintf("http://%s.%s.svc:%d", serviceName, namespace, req.DeclaredPort),
		ServiceName:  serviceName,
		Protocol:     models.ProtocolHTTP,
		Status:       models.EndpointInactive,
		RegisteredAt: time.Now().UTC(),
	}
	return DeploymentPlan{Manifests: manifests, Deployment: deployment, Endpoint: endpoint}, nil
}

func (m Manager) RecordReady(ctx context.Context, plan DeploymentPlan) error {
	deployment := plan.Deployment
	deployment.Status = models.DeploymentReady
	if err := m.Repo.SaveDeployment(ctx, &deployment); err != nil {
		return err
	}
	endpoint := plan.Endpoint
	endpoint.Status = models.EndpointActive

	// **NEW**: Explicitly save the endpoint
	if err := m.Repo.SaveEndpoint(ctx, &endpoint); err != nil {
		return err
	}

	// Register with the manager
	if err := m.Repo.RegisterEndpoint(ctx, endpoint); err != nil {
		return err
	}
	if m.Publisher != nil {
		env, err := events.NewEnvelope(events.DeploymentReady, deployment.SubmissionID, "deployment-manager", events.DeploymentReadyData{
			PodName:     deployment.PodName,
			InternalURL: endpoint.InternalURL,
			ServiceName: endpoint.ServiceName,
		})
		if err != nil {
			return err
		}
		return m.Publisher.Publish(ctx, env)
	}
	return nil
}

func (m Manager) Teardown(ctx context.Context, submissionID, reason string) error {
	if err := m.Repo.DeregisterEndpoint(ctx, submissionID); err != nil && err != store.ErrNotFound {
		return err
	}
	if m.Publisher != nil {
		env, err := events.NewEnvelope(events.TeardownCompleted, submissionID, "deployment-manager", map[string]string{"reason": reason})
		if err != nil {
			return err
		}
		return m.Publisher.Publish(ctx, env)
	}
	return nil
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func defaultInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}
