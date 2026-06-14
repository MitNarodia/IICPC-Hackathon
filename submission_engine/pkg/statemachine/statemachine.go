package statemachine

import (
	"fmt"

	"github.com/iicpc/track1/submission-engine/pkg/models"
)

type Event string

const (
	EventSubmissionCreated  Event = "submission.created"
	EventUploadSucceeded    Event = "upload.succeeded"
	EventUploadFailed       Event = "upload.failed"
	EventValidationStarted  Event = "validation.started"
	EventValidationPassed   Event = "validation.passed"
	EventValidationFailed   Event = "validation.failed"
	EventBuildStarted       Event = "build.started"
	EventBuildSucceeded     Event = "build.succeeded"
	EventBuildFailed        Event = "build.failed"
	EventScanStarted        Event = "scan.started"
	EventScanPassed         Event = "scan.passed"
	EventScanFailed         Event = "scan.failed"
	EventDeploymentStarted  Event = "deployment.started"
	EventDeploymentPodReady Event = "deployment.pod_ready"
	EventDeploymentFailed   Event = "deployment.failed"
	EventHealthReady        Event = "health.ready"
	EventHealthFailed       Event = "health.failed"
	EventHealthDegraded     Event = "health.degraded"
	EventHealthRecovered    Event = "health.recovered"
	EventTeardownCompleted  Event = "teardown.completed"
)

type TransitionRecord struct {
	From  models.SubmissionStatus
	Event Event
	To    models.SubmissionStatus
}

var transitions = map[models.SubmissionStatus]map[Event]models.SubmissionStatus{
	"": {
		EventSubmissionCreated: models.StatusCreated,
	},
	models.StatusCreated: {
		EventUploadSucceeded: models.StatusUploaded,
		EventUploadFailed:    models.StatusUploadFailed,
	},
	models.StatusUploaded: {
		EventValidationStarted: models.StatusValidating,
	},
	models.StatusValidating: {
		EventValidationPassed: models.StatusValidated,
		EventValidationFailed: models.StatusValidationFailed,
	},
	models.StatusValidated: {
		EventBuildStarted: models.StatusBuilding,
	},
	models.StatusBuilding: {
		EventBuildSucceeded: models.StatusBuilt,
		EventBuildFailed:    models.StatusBuildFailed,
	},
	models.StatusBuilt: {
		EventScanStarted: models.StatusScanning,
	},
	models.StatusScanning: {
		EventScanPassed: models.StatusScanned,
		EventScanFailed: models.StatusScanFailed,
	},
	models.StatusScanned: {
		EventDeploymentStarted: models.StatusDeploying,
	},
	models.StatusDeploying: {
		EventDeploymentPodReady: models.StatusHealthCheck,
		EventDeploymentFailed:   models.StatusDeployFailed,
	},
	models.StatusHealthCheck: {
		EventHealthReady:  models.StatusReady,
		EventHealthFailed: models.StatusHealthFailed,
	},
	models.StatusReady: {
		EventHealthDegraded:    models.StatusDegraded,
		EventTeardownCompleted: models.StatusTerminated,
	},
	models.StatusDegraded: {
		EventHealthRecovered:   models.StatusReady,
		EventTeardownCompleted: models.StatusTerminated,
	},
}

func Transition(current models.SubmissionStatus, event Event) (models.SubmissionStatus, error) {
	nextByEvent, ok := transitions[current]
	if !ok {
		return "", fmt.Errorf("status %s is terminal or unknown", current)
	}
	next, ok := nextByEvent[event]
	if !ok {
		return "", fmt.Errorf("illegal transition from %s on %s", current, event)
	}
	return next, nil
}

func CanTransition(current models.SubmissionStatus, event Event) bool {
	_, err := Transition(current, event)
	return err == nil
}

func AllTransitions() []TransitionRecord {
	out := make([]TransitionRecord, 0)
	for from, byEvent := range transitions {
		for event, to := range byEvent {
			out = append(out, TransitionRecord{From: from, Event: event, To: to})
		}
	}
	return out
}
