package statemachine

import (
	"testing"

	"github.com/iicpc/track1/submission-engine/pkg/models"
)

func TestHappyPath(t *testing.T) {
	status := models.SubmissionStatus("")
	events := []Event{
		EventSubmissionCreated,
		EventUploadSucceeded,
		EventValidationStarted,
		EventValidationPassed,
		EventBuildStarted,
		EventBuildSucceeded,
		EventScanStarted,
		EventScanPassed,
		EventDeploymentStarted,
		EventDeploymentPodReady,
		EventHealthReady,
	}
	for _, event := range events {
		next, err := Transition(status, event)
		if err != nil {
			t.Fatalf("Transition(%s, %s) error = %v", status, event, err)
		}
		status = next
	}
	if status != models.StatusReady {
		t.Fatalf("final status = %s, want READY", status)
	}
}

func TestIllegalTransitionRejected(t *testing.T) {
	if _, err := Transition(models.StatusCreated, EventBuildSucceeded); err == nil {
		t.Fatal("expected illegal transition error")
	}
	if CanTransition(models.StatusBuildFailed, EventHealthReady) {
		t.Fatal("terminal state must reject transitions")
	}
}

func TestDegradedRecoveryAndTeardown(t *testing.T) {
	next, err := Transition(models.StatusReady, EventHealthDegraded)
	if err != nil {
		t.Fatalf("degrade: %v", err)
	}
	if next != models.StatusDegraded {
		t.Fatalf("next = %s, want DEGRADED", next)
	}
	next, err = Transition(next, EventHealthRecovered)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if next != models.StatusReady {
		t.Fatalf("next = %s, want READY", next)
	}
	next, err = Transition(next, EventTeardownCompleted)
	if err != nil {
		t.Fatalf("teardown: %v", err)
	}
	if next != models.StatusTerminated {
		t.Fatalf("next = %s, want TERMINATED", next)
	}
}
