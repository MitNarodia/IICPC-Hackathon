package service

import (
	"context"
	"testing"

	"github.com/iicpc/track1/submission-engine/pkg/events"
	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/security"
	"github.com/iicpc/track1/submission-engine/pkg/store"
)

func TestPlanProducesHardenedManifests(t *testing.T) {
	manager := Manager{}
	plan, err := manager.Plan(DeployRequest{
		SubmissionID: models.MustUUIDv7(),
		ImageRef:     "registry/submission@sha256:abc",
		Entrypoint:   "/app/bot",
		DeclaredPort: 8080,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if err := security.ValidateSandboxPod(plan.Manifests.Pod); err != nil {
		t.Fatalf("sandbox pod not hardened: %v", err)
	}
	if plan.Endpoint.Status != models.EndpointInactive {
		t.Fatalf("endpoint must stay inactive until health gate")
	}
}

func TestRecordReadyRegistersEndpointAndPublishes(t *testing.T) {
	repo := store.NewMemoryStore()
	bus := events.NewInMemoryBus()
	published := 0
	bus.Subscribe(events.DeploymentReady, func(context.Context, events.Envelope) error {
		published++
		return nil
	})
	manager := Manager{Repo: repo, Publisher: bus}
	plan, err := manager.Plan(DeployRequest{
		SubmissionID: models.MustUUIDv7(),
		ImageRef:     "registry/submission@sha256:abc",
		DeclaredPort: 8080,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RecordReady(context.Background(), plan); err != nil {
		t.Fatalf("RecordReady() error = %v", err)
	}
	endpoint, err := repo.LookupEndpoint(context.Background(), plan.Deployment.SubmissionID)
	if err != nil {
		t.Fatalf("LookupEndpoint() error = %v", err)
	}
	if endpoint.Status != models.EndpointActive || published != 1 {
		t.Fatalf("endpoint=%#v published=%d", endpoint, published)
	}
}

func TestTeardownDeregistersFirst(t *testing.T) {
	repo := store.NewMemoryStore()
	manager := Manager{Repo: repo, Publisher: events.NewInMemoryBus()}
	plan, err := manager.Plan(DeployRequest{
		SubmissionID: models.MustUUIDv7(),
		ImageRef:     "registry/submission@sha256:abc",
		DeclaredPort: 8080,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RecordReady(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := manager.Teardown(context.Background(), plan.Deployment.SubmissionID, "test"); err != nil {
		t.Fatalf("Teardown() error = %v", err)
	}
	endpoint, err := repo.LookupEndpoint(context.Background(), plan.Deployment.SubmissionID)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Status != models.EndpointInactive {
		t.Fatalf("endpoint status = %s, want INACTIVE", endpoint.Status)
	}
}
