package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	result := Prober{Timeout: time.Second}.ProbeHTTP(context.Background(), server.URL)
	if !result.Healthy {
		t.Fatalf("expected healthy probe: %#v", result)
	}
	if result.LatencyMS <= 0 {
		t.Fatalf("latency must be recorded: %#v", result)
	}
}

func TestClassifierThresholds(t *testing.T) {
	classifier := NewClassifier(2, 2, 100)
	if state := classifier.Observe(ProbeResult{Healthy: true, LatencyMS: 1}); state != HealthUnknown {
		t.Fatalf("state after first success = %s", state)
	}
	if state := classifier.Observe(ProbeResult{Healthy: true, LatencyMS: 1}); state != HealthReady {
		t.Fatalf("state after second success = %s", state)
	}
	if state := classifier.Observe(ProbeResult{Healthy: false}); state != HealthReady {
		t.Fatalf("state after first failure = %s", state)
	}
	if state := classifier.Observe(ProbeResult{Healthy: false}); state != HealthDegraded {
		t.Fatalf("state after second failure = %s", state)
	}
}
