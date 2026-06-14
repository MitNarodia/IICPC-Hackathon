package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/statemachine"
)

var ErrNotFound = errors.New("not found")

type MemoryStore struct {
	mu          sync.RWMutex
	submissions map[string]*models.Submission
	builds      map[string]*models.Build
	deployments map[string]*models.Deployment
	endpoints   map[string]*models.Endpoint
	health      map[string][]models.HealthSample
	audit       []models.AuditLog
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		submissions: make(map[string]*models.Submission),
		builds:      make(map[string]*models.Build),
		deployments: make(map[string]*models.Deployment),
		endpoints:   make(map[string]*models.Endpoint),
		health:      make(map[string][]models.HealthSample),
	}
}

func (s *MemoryStore) CreateSubmission(_ context.Context, submission *models.Submission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.submissions[submission.ID]; exists {
		return fmt.Errorf("submission %s already exists", submission.ID)
	}
	cp := *submission
	s.submissions[submission.ID] = &cp
	return nil
}

func (s *MemoryStore) GetSubmission(_ context.Context, id string) (*models.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	submission, ok := s.submissions[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *submission
	return &cp, nil
}

func (s *MemoryStore) ListSubmissions(_ context.Context) ([]*models.Submission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*models.Submission, 0, len(s.submissions))
	for _, submission := range s.submissions {
		cp := *submission
		out = append(out, &cp)
	}
	return out, nil
}

func (s *MemoryStore) UpdateSubmission(_ context.Context, submission *models.Submission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.submissions[submission.ID]; !ok {
		return ErrNotFound
	}
	cp := *submission
	cp.UpdatedAt = time.Now().UTC()
	s.submissions[submission.ID] = &cp
	return nil
}

func (s *MemoryStore) TransitionSubmission(_ context.Context, id string, event statemachine.Event, actor string, detail map[string]interface{}) (*models.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	submission, ok := s.submissions[id]
	if !ok {
		return nil, ErrNotFound
	}
	prev := submission.Status
	next, err := statemachine.Transition(prev, event)
	if err != nil {
		return nil, err
	}
	cp := *submission
	cp.Status = next
	cp.UpdatedAt = time.Now().UTC()
	s.submissions[id] = &cp

	auditID, err := models.NewUUIDv7()
	if err != nil {
		return nil, err
	}
	s.audit = append(s.audit, models.AuditLog{
		ID:           auditID,
		SubmissionID: id,
		Actor:        actor,
		Action:       string(event),
		PrevState:    prev,
		NewState:     next,
		Detail:       detail,
		CreatedAt:    time.Now().UTC(),
	})
	out := cp
	return &out, nil
}

func (s *MemoryStore) SaveBuild(_ context.Context, build *models.Build) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *build
	s.builds[build.SubmissionID] = &cp
	return nil
}

func (s *MemoryStore) GetBuild(_ context.Context, submissionID string) (*models.Build, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	build, ok := s.builds[submissionID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *build
	return &cp, nil
}

func (s *MemoryStore) SaveDeployment(_ context.Context, deployment *models.Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *deployment
	s.deployments[deployment.SubmissionID] = &cp
	return nil
}

func (s *MemoryStore) GetDeployment(_ context.Context, submissionID string) (*models.Deployment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	deployment, ok := s.deployments[submissionID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *deployment
	return &cp, nil
}

func (s *MemoryStore) SaveEndpoint(_ context.Context, endpoint *models.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *endpoint
	s.endpoints[endpoint.SubmissionID] = &cp
	return nil
}

func (s *MemoryStore) GetEndpoint(_ context.Context, submissionID string) (*models.Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	endpoint, ok := s.endpoints[submissionID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *endpoint
	return &cp, nil
}

func (s *MemoryStore) SetEndpointStatus(_ context.Context, submissionID string, status models.EndpointStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	endpoint, ok := s.endpoints[submissionID]
	if !ok {
		return ErrNotFound
	}
	cp := *endpoint
	cp.Status = status
	s.endpoints[submissionID] = &cp
	return nil
}

func (s *MemoryStore) AddHealthSample(_ context.Context, sample models.HealthSample) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health[sample.SubmissionID] = append(s.health[sample.SubmissionID], sample)
	return nil
}

func (s *MemoryStore) ListHealthSamples(_ context.Context, submissionID string, limit int) ([]models.HealthSample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	samples := s.health[submissionID]
	if limit <= 0 || limit > len(samples) {
		limit = len(samples)
	}
	start := len(samples) - limit
	out := append([]models.HealthSample(nil), samples[start:]...)
	return out, nil
}

func (s *MemoryStore) WriteAudit(_ context.Context, entry models.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		id, err := models.NewUUIDv7()
		if err != nil {
			return err
		}
		entry.ID = id
	}
	entry.CreatedAt = time.Now().UTC()
	s.audit = append(s.audit, entry)
	return nil
}

func (s *MemoryStore) ListAudit(_ context.Context, submissionID string) ([]models.AuditLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.AuditLog, 0)
	for _, entry := range s.audit {
		if submissionID == "" || entry.SubmissionID == submissionID {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *MemoryStore) RegisterEndpoint(ctx context.Context, endpoint models.Endpoint) error {
	endpoint.Status = models.EndpointActive
	return s.SaveEndpoint(ctx, &endpoint)
}

func (s *MemoryStore) DeregisterEndpoint(ctx context.Context, submissionID string) error {
	return s.SetEndpointStatus(ctx, submissionID, models.EndpointInactive)
}

func (s *MemoryStore) LookupEndpoint(ctx context.Context, submissionID string) (*models.Endpoint, error) {
	return s.GetEndpoint(ctx, submissionID)
}
