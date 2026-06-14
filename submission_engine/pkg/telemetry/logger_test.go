package telemetry

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestLoggerWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger("svc", &buf)
	logger.Info("created", Field{"submission_id": "sub-1"})
	var payload map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("log is not JSON: %v", err)
	}
	if payload["service"] != "svc" || payload["submission_id"] != "sub-1" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestCounterSetSnapshot(t *testing.T) {
	counters := NewCounterSet()
	counters.Inc("ready")
	counters.Add("ready", 2)
	if got := counters.Snapshot()["ready"]; got != 3 {
		t.Fatalf("ready = %d, want 3", got)
	}
}
