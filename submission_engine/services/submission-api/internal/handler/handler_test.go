package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iicpc/track1/submission-engine/pkg/events"
	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/store"
	"github.com/iicpc/track1/submission-engine/services/submission-api/internal/service"
)

func TestCreateSubmission(t *testing.T) {
	repo := store.NewMemoryStore()
	bus := events.NewInMemoryBus()
	h := Handler{Service: service.Service{
		Repo:       repo,
		UploadURLs: store.StaticUploadURLProvider{BaseURL: "s3://raw-uploads"},
		Publisher:  bus,
	}}
	mux := http.NewServeMux()
	h.Register(mux)

	body := map[string]interface{}{
		"contestant_id":   models.MustUUIDv7(),
		"language":        "go",
		"submission_type": "source",
		"entrypoint":      "/app/bot",
		"declared_port":   8080,
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/submissions", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp service.CreateSubmissionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != models.StatusCreated || resp.UploadURL == "" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestCreateSubmissionRejectsBadPort(t *testing.T) {
	repo := store.NewMemoryStore()
	h := Handler{Service: service.Service{
		Repo:       repo,
		UploadURLs: store.StaticUploadURLProvider{},
		Publisher:  events.NewInMemoryBus(),
	}}
	mux := http.NewServeMux()
	h.Register(mux)

	body := []byte(`{"contestant_id":"` + models.MustUUIDv7() + `","language":"go","submission_type":"source","entrypoint":"bot","declared_port":80}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/submissions", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content-type = %q", got)
	}
}
