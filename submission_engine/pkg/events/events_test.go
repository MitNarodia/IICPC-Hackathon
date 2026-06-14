package events

import (
	"context"
	"testing"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	env, err := NewEnvelope(BuildSucceeded, "018fd6c2-5a6b-7abc-8def-111111111111", "build-service", BuildSucceededData{
		ImageRef:    "registry/submission@sha256:abc",
		ImageDigest: "sha256:abc",
	})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	payload, err := Marshal(env)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	got, err := Unmarshal(payload)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Topic() != "build.succeeded" || got.Key() != env.SubmissionID {
		t.Fatalf("unexpected topic/key: %s/%s", got.Topic(), got.Key())
	}
	var data BuildSucceededData
	if err := got.DecodeData(&data); err != nil {
		t.Fatalf("DecodeData() error = %v", err)
	}
	if data.ImageDigest != "sha256:abc" {
		t.Fatalf("ImageDigest = %q", data.ImageDigest)
	}
}

func TestInMemoryBusDedupe(t *testing.T) {
	bus := NewInMemoryBus()
	calls := 0
	bus.Subscribe(HealthReady, func(context.Context, Envelope) error {
		calls++
		return nil
	})
	env, err := NewEnvelope(HealthReady, "018fd6c2-5a6b-7abc-8def-111111111111", "health-monitor", HealthReadyData{InternalURL: "http://x"})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := bus.Publish(context.Background(), env); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
