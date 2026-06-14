package store

import (
	"context"

	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/statemachine"
)

type SubmissionRepository interface {
	CreateSubmission(ctx context.Context, submission *models.Submission) error
	GetSubmission(ctx context.Context, id string) (*models.Submission, error)
	ListSubmissions(ctx context.Context) ([]*models.Submission, error)
	UpdateSubmission(ctx context.Context, submission *models.Submission) error
	TransitionSubmission(ctx context.Context, id string, event statemachine.Event, actor string, detail map[string]interface{}) (*models.Submission, error)
}

type BuildRepository interface {
	SaveBuild(ctx context.Context, build *models.Build) error
	GetBuild(ctx context.Context, submissionID string) (*models.Build, error)
}

type DeploymentRepository interface {
	SaveDeployment(ctx context.Context, deployment *models.Deployment) error
	GetDeployment(ctx context.Context, submissionID string) (*models.Deployment, error)
	SaveEndpoint(ctx context.Context, endpoint *models.Endpoint) error
	GetEndpoint(ctx context.Context, submissionID string) (*models.Endpoint, error)
	SetEndpointStatus(ctx context.Context, submissionID string, status models.EndpointStatus) error
}

type HealthRepository interface {
	AddHealthSample(ctx context.Context, sample models.HealthSample) error
	ListHealthSamples(ctx context.Context, submissionID string, limit int) ([]models.HealthSample, error)
}

type AuditRepository interface {
	WriteAudit(ctx context.Context, entry models.AuditLog) error
	ListAudit(ctx context.Context, submissionID string) ([]models.AuditLog, error)
}

type Registry interface {
	RegisterEndpoint(ctx context.Context, endpoint models.Endpoint) error
	DeregisterEndpoint(ctx context.Context, submissionID string) error
	LookupEndpoint(ctx context.Context, submissionID string) (*models.Endpoint, error)
}

type UploadURLProvider interface {
	PresignUploadURL(ctx context.Context, submissionID string) (string, error)
}
