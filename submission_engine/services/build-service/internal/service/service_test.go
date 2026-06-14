package service

import (
	"testing"

	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/services/build-service/internal/profiles"
)

func TestPlannerCreatesRootlessJob(t *testing.T) {
	plan, err := Planner{Namespace: "track1-build"}.Plan("sub-1", profiles.RenderRequest{
		Language:       models.LanguageGo,
		SubmissionType: models.SubmissionTypeSource,
		DeclaredPort:   8080,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Dockerfile == "" || plan.LogsURI == "" {
		t.Fatalf("incomplete plan: %#v", plan)
	}
	spec := plan.Job["spec"].(map[string]interface{})
	if spec["ttlSecondsAfterFinished"] == nil {
		t.Fatalf("job must set ttlSecondsAfterFinished: %#v", plan.Job)
	}
}
