package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/iicpc/track1/submission-engine/pkg/store"
	"github.com/iicpc/track1/submission-engine/services/submission-api/internal/service"
)

type Handler struct {
	Service service.Service
}

func (h Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.health)
	mux.HandleFunc("/readyz", h.health)
	mux.HandleFunc("/v1/submissions", h.submissions)
	mux.HandleFunc("/v1/submissions/", h.submissionByID)
}

func (h Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (h Handler) submissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req service.CreateSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid JSON", err.Error())
		return
	}
	resp, err := h.Service.CreateSubmission(r.Context(), req)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid submission", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h Handler) submissionByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/submissions/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeProblem(w, http.StatusNotFound, "not found", "")
		return
	}
	id := parts[0]
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		h.getSubmission(w, r, id)
	case len(parts) == 2 && parts[1] == "teardown" && r.Method == http.MethodPost:
		h.teardown(w, r, id)
	case len(parts) == 2 && parts[1] == "logs" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]string{"logs_url": "s3://build-logs/" + id})
	case len(parts) == 2 && parts[1] == "health" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"healthy": false, "latency_ms": 0})
	case len(parts) == 2 && parts[1] == "deployment" && r.Method == http.MethodGet:
		h.getDeployment(w, r, id)
	default:
		writeProblem(w, http.StatusNotFound, "not found", "")
	}
}

func (h Handler) getSubmission(w http.ResponseWriter, r *http.Request, id string) {
	view, err := h.Service.GetSubmission(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeProblem(w, status, "submission lookup failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h Handler) getDeployment(w http.ResponseWriter, r *http.Request, id string) {
	view, err := h.Service.GetDeploymentInfo(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeProblem(w, status, "deployment lookup failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (h Handler) teardown(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.Service.RequestTeardown(r.Context(), id, "api request"); err != nil {
		writeProblem(w, http.StatusBadRequest, "teardown failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"type":   "about:blank",
		"title":  title,
		"status": status,
		"detail": detail,
	})
}
