// Package track3 provides a client for reporting sandbox metrics to Track 3's
// ingestion service. Used by health-monitor to bridge Track 1 → Track 3.
package track3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// SandboxSample is the payload shape Track 3's /v1/track1/sandbox expects.
type SandboxSample struct {
	RunID        string  `json:"run_id"`
	SubmissionID string  `json:"submission_id"`
	Source       string  `json:"source"`
	Seq          uint64  `json:"seq"`
	PodName      string  `json:"pod_name"`
	Namespace    string  `json:"namespace"`
	CPUm         float64 `json:"cpu_millicores"`
	CPULimitm    float64 `json:"cpu_limit_millicores"`
	MemBytes     uint64  `json:"memory_bytes"`
	MemLimit     uint64  `json:"memory_limit_bytes"`
	OpenFDs      uint32  `json:"open_fds"`
	ActiveConns  uint32  `json:"active_connections"`
	Health       string  `json:"health"`
	OOMKilled    bool    `json:"oom_killed"`
	RestartCount uint32  `json:"restart_count"`
	SampleTS     int64   `json:"sample_ts"`
}

// Reporter sends sandbox metrics to Track 3's ingestion service.
type Reporter struct {
	baseURL string
	runID   string
	client  *http.Client
	seq     atomic.Uint64
}

// NewReporter creates a Track 3 reporter. Returns nil if TRACK3_INGEST_URL is
// not set (integration disabled).
func NewReporter() *Reporter {
	url := os.Getenv("TRACK3_INGEST_URL")
	if url == "" {
		return nil
	}
	runID := os.Getenv("TRACK3_RUN_ID")
	if runID == "" {
		runID = "run-default"
	}
	return &Reporter{
		baseURL: url,
		runID:   runID,
		client:  &http.Client{Timeout: 3 * time.Second},
	}
}

// ReportSandbox sends one sandbox sample to Track 3. Best-effort; errors are
// silently dropped (the health-monitor should not fail because Track 3 is down).
func (r *Reporter) ReportSandbox(ctx context.Context, submissionID string, sample SandboxSample) error {
	if r == nil {
		return nil
	}
	sample.RunID = r.runID
	sample.SubmissionID = submissionID
	sample.Seq = r.seq.Add(1)
	if sample.Source == "" {
		sample.Source = "health-monitor"
	}
	if sample.SampleTS == 0 {
		sample.SampleTS = time.Now().UnixNano()
	}

	body, err := json.Marshal(sample)
	if err != nil {
		return fmt.Errorf("track3 marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/track1/sandbox", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}
