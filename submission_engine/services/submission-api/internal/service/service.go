package service

import (
	"context"
	"errors"

	"github.com/iicpc/track1/submission-engine/pkg/events"
	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/store"
)

type Repository interface {
	store.SubmissionRepository
	store.DeploymentRepository
	store.HealthRepository
}

type Service struct {
	Repo       Repository
	UploadURLs store.UploadURLProvider
	Publisher  events.Publisher
}

type CreateSubmissionRequest struct {
	ContestantID   string                 `json:"contestant_id"`
	Language       models.Language        `json:"language"`
	SubmissionType models.SubmissionType  `json:"submission_type"`
	Entrypoint     string                 `json:"entrypoint"`
	DeclaredPort   int                    `json:"declared_port"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type CreateSubmissionResponse struct {
	ID        string                  `json:"id"`
	UploadURL string                  `json:"upload_url"`
	Status    models.SubmissionStatus `json:"status"`
}

type SubmissionView struct {
	ID        string                  `json:"id"`
	Status    models.SubmissionStatus `json:"status"`
	Endpoint  string                  `json:"endpoint,omitempty"`
	CreatedAt string                  `json:"created_at"`
	UpdatedAt string                  `json:"updated_at"`
}

func (s Service) CreateSubmission(ctx context.Context, req CreateSubmissionRequest) (CreateSubmissionResponse, error) {
	if s.Repo == nil || s.UploadURLs == nil || s.Publisher == nil {
		return CreateSubmissionResponse{}, errors.New("service dependencies are required")
	}
	submission, err := models.NewSubmission(req.ContestantID, req.Language, req.SubmissionType, req.Entrypoint, req.DeclaredPort, req.Metadata)
	if err != nil {
		return CreateSubmissionResponse{}, err
	}
	uploadURL, err := s.UploadURLs.PresignUploadURL(ctx, submission.ID)
	if err != nil {
		return CreateSubmissionResponse{}, err
	}
	if err := s.Repo.CreateSubmission(ctx, submission); err != nil {
		return CreateSubmissionResponse{}, err
	}
	env, err := events.NewEnvelope(events.SubmissionCreated, submission.ID, "submission-api", events.SubmissionCreatedData{
		ContestantID:   submission.ContestantID,
		Language:       string(submission.Language),
		SubmissionType: string(submission.Type),
		UploadURL:      uploadURL,
	})
	if err != nil {
		return CreateSubmissionResponse{}, err
	}
	if err := s.Publisher.Publish(ctx, env); err != nil {
		return CreateSubmissionResponse{}, err
	}
	return CreateSubmissionResponse{ID: submission.ID, UploadURL: uploadURL, Status: submission.Status}, nil
}

func (s Service) GetSubmission(ctx context.Context, id string) (SubmissionView, error) {
	submission, err := s.Repo.GetSubmission(ctx, id)
	if err != nil {
		return SubmissionView{}, err
	}
	view := SubmissionView{
		ID:        submission.ID,
		Status:    submission.Status,
		CreatedAt: submission.CreatedAt.Format(timeFormat),
		UpdatedAt: submission.UpdatedAt.Format(timeFormat),
	}
	endpoint, err := s.Repo.GetEndpoint(ctx, id)
	if err == nil && endpoint.Status == models.EndpointActive {
		view.Endpoint = endpoint.InternalURL
	}
	return view, nil
}

func (s Service) RequestTeardown(ctx context.Context, id, reason string) error {
	if err := models.ValidateUUID(id); err != nil {
		return err
	}
	env, err := events.NewEnvelope(events.TeardownRequested, id, "submission-api", events.TeardownRequestedData{Reason: reason})
	if err != nil {
		return err
	}
	return s.Publisher.Publish(ctx, env)
}

// DeploymentView is the response for GET /v1/submissions/{id}/deployment.
// It provides all the information needed to discover a deployed contestant endpoint.
type DeploymentView struct {
	SubmissionID   string                  `json:"submission_id"`
	Status         models.DeploymentStatus `json:"status"`
	Endpoint       string                  `json:"endpoint,omitempty"`
	Deployment     *models.Deployment      `json:"deployment,omitempty"`
	EndpointDetail *models.Endpoint        `json:"endpoint_detail,omitempty"`
}

// GetDeploymentInfo returns the deployment status and endpoint for a submission.
// This is the primary discovery mechanism for Track 1 → Track 2 integration.
func (s Service) GetDeploymentInfo(ctx context.Context, submissionID string) (DeploymentView, error) {
	if err := models.ValidateUUID(submissionID); err != nil {
		return DeploymentView{}, err
	}
	view := DeploymentView{SubmissionID: submissionID}

	deployment, err := s.Repo.GetDeployment(ctx, submissionID)
	if err != nil {
		// No deployment yet — return a minimal view with PENDING status.
		if errors.Is(err, store.ErrNotFound) {
			view.Status = models.DeploymentPending
			return view, nil
		}
		return DeploymentView{}, err
	}
	view.Status = deployment.Status
	view.Deployment = deployment

	endpoint, err := s.Repo.GetEndpoint(ctx, submissionID)
	if err == nil {
		view.EndpointDetail = endpoint
		if endpoint.Status == models.EndpointActive {
			view.Endpoint = endpoint.InternalURL
		}
	}

	return view, nil
}

const timeFormat = "2006-01-02T15:04:05Z07:00"
