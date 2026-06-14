//go:build integration

// Package integration holds black-box tests that exercise the whole Track 3
// pipeline against a RUNNING stack (docker compose up). They are excluded from
// normal `go test` by the build tag; run them with:
//
//	go test -tags=integration ./test/integration/...
//
// Env overrides: INGEST_URL (default :8081), LEADERBOARD_URL (default :8080).
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// TestPipelineEndToEnd feeds a few seconds of order acks through ingestion and
// asserts the submission eventually surfaces on the leaderboard with a positive
// composite score — proving ingestion → stream-processor → scoring →
// leaderboard all wired up and flowing.
func TestPipelineEndToEnd(t *testing.T) {
	ingest := envOr("INGEST_URL", "http://localhost:8081")
	board := envOr("LEADERBOARD_URL", "http://localhost:8080")
	runID := fmt.Sprintf("it-%d", time.Now().Unix())
	subID := "sub-it-01"

	client := &http.Client{Timeout: 5 * time.Second}

	// Produce ~8s of steady, correct traffic so a tumbling window closes and a
	// score is emitted.
	var seq uint64
	feedUntil := time.Now().Add(8 * time.Second)
	for time.Now().Before(feedUntil) {
		now := time.Now().UnixNano()
		batch := make([]*events.Envelope, 0, 200)
		for i := 0; i < 200; i++ {
			seq++
			ack := events.OrderAck{
				OrderID: seq, Accepted: true, RecvTS: now, AckLatencyUS: 150,
			}
			env, err := events.NewEnvelope(runID, subID, "integration", seq, events.TypeOrderAck, ack)
			if err != nil {
				t.Fatalf("build envelope: %v", err)
			}
			batch = append(batch, env)
		}
		postEvents(t, client, ingest, batch)
		time.Sleep(250 * time.Millisecond)
	}

	// Poll the leaderboard until our submission appears with a score.
	type entry struct {
		SubmissionID string  `json:"submission_id"`
		Composite    float64 `json:"composite"`
		TPS          float64 `json:"tps"`
	}
	type boardResp struct {
		Entries []entry `json:"entries"`
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(board + "/v1/leaderboard?run=" + runID)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		var b boardResp
		if resp.StatusCode == http.StatusOK {
			_ = json.NewDecoder(resp.Body).Decode(&b)
		}
		resp.Body.Close()
		for _, e := range b.Entries {
			if e.SubmissionID == subID && e.Composite > 0 {
				t.Logf("leaderboard hit: %s composite=%.2f tps=%.0f", e.SubmissionID, e.Composite, e.TPS)
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("submission %s never appeared on leaderboard for run %s within timeout", subID, runID)
}

func postEvents(t *testing.T, client *http.Client, base string, batch []*events.Envelope) {
	t.Helper()
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("marshal batch: %v", err)
	}
	resp, err := client.Post(base+"/v1/events", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("ingestion returned %d, want 202", resp.StatusCode)
	}
}
