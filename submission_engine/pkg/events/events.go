package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/iicpc/track1/submission-engine/pkg/models"
)

type Type string

const (
	SubmissionCreated   Type = "submission.created"
	SubmissionUploaded  Type = "submission.uploaded"
	ValidationFailed    Type = "validation.failed"
	BuildRequested      Type = "build.requested"
	BuildSucceeded      Type = "build.succeeded"
	BuildFailed         Type = "build.failed"
	ScanPassed          Type = "scan.passed"
	ScanFailed          Type = "scan.failed"
	DeploymentRequested Type = "deployment.requested"
	DeploymentReady     Type = "deployment.ready"
	DeploymentFailed    Type = "deployment.failed"
	HealthReady         Type = "health.ready"
	HealthDegraded      Type = "health.degraded"
	HealthRecovered     Type = "health.recovered"
	TeardownRequested   Type = "teardown.requested"
	TeardownCompleted   Type = "teardown.completed"
)

type Envelope struct {
	EventID       string          `json:"event_id"`
	Type          Type            `json:"type"`
	SubmissionID  string          `json:"submission_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Producer      string          `json:"producer"`
	SchemaVersion int             `json:"schema_version"`
	Data          json.RawMessage `json:"data"`
}

func NewEnvelope(eventType Type, submissionID, producer string, data interface{}) (Envelope, error) {
	eventID, err := models.NewUUIDv7()
	if err != nil {
		return Envelope{}, err
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID:       eventID,
		Type:          eventType,
		SubmissionID:  submissionID,
		OccurredAt:    time.Now().UTC(),
		Producer:      producer,
		SchemaVersion: 1,
		Data:          payload,
	}, nil
}

func (e Envelope) Topic() string {
	return string(e.Type)
}

func (e Envelope) Key() string {
	return e.SubmissionID
}

func (e Envelope) DecodeData(v interface{}) error {
	if len(e.Data) == 0 {
		return nil
	}
	return json.Unmarshal(e.Data, v)
}

func Marshal(e Envelope) ([]byte, error) {
	if e.SchemaVersion == 0 {
		return nil, fmt.Errorf("schema_version is required")
	}
	if e.EventID == "" || e.Type == "" || e.SubmissionID == "" || e.Producer == "" {
		return nil, fmt.Errorf("event_id, type, submission_id, and producer are required")
	}
	return json.Marshal(e)
}

func Unmarshal(payload []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(payload, &e); err != nil {
		return Envelope{}, err
	}
	if e.SchemaVersion != 1 {
		return Envelope{}, fmt.Errorf("unsupported schema_version %d", e.SchemaVersion)
	}
	if e.Type == "" || e.SubmissionID == "" {
		return Envelope{}, fmt.Errorf("event type and submission_id are required")
	}
	return e, nil
}
