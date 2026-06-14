package store

import (
	"context"
	"testing"

	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/statemachine"
)

func TestTransitionSubmissionWritesAudit(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryStore()
	sub, err := models.NewSubmission(models.MustUUIDv7(), models.LanguageCPP, models.SubmissionTypeSource, "./bot", 8080, nil)
	if err != nil {
		t.Fatalf("NewSubmission() error = %v", err)
	}
	if err := repo.CreateSubmission(ctx, sub); err != nil {
		t.Fatalf("CreateSubmission() error = %v", err)
	}
	got, err := repo.TransitionSubmission(ctx, sub.ID, statemachine.EventUploadSucceeded, "upload-service", nil)
	if err != nil {
		t.Fatalf("TransitionSubmission() error = %v", err)
	}
	if got.Status != models.StatusUploaded {
		t.Fatalf("status = %s, want UPLOADED", got.Status)
	}
	audit, err := repo.ListAudit(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(audit) != 1 || audit[0].PrevState != models.StatusCreated || audit[0].NewState != models.StatusUploaded {
		t.Fatalf("unexpected audit: %#v", audit)
	}
}

func TestStaticUploadURLProvider(t *testing.T) {
	provider := StaticUploadURLProvider{BaseURL: "s3://raw-uploads"}
	url, err := provider.PresignUploadURL(context.Background(), "abc")
	if err != nil {
		t.Fatalf("PresignUploadURL() error = %v", err)
	}
	if url != "s3://raw-uploads/abc/artifact" {
		t.Fatalf("url = %q", url)
	}
}
