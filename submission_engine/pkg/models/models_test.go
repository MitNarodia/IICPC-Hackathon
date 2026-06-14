package models

import (
	"strings"
	"testing"
)

func TestNewUUIDv7(t *testing.T) {
	id, err := NewUUIDv7()
	if err != nil {
		t.Fatalf("NewUUIDv7() error = %v", err)
	}
	if err := ValidateUUID(id); err != nil {
		t.Fatalf("generated invalid uuid: %v", err)
	}
	if got := id[14]; got != '7' {
		t.Fatalf("uuid version = %q, want 7 in %s", got, id)
	}
}

func TestNewSubmissionValidation(t *testing.T) {
	contestant := MustUUIDv7()
	sub, err := NewSubmission(contestant, LanguageGo, SubmissionTypeSource, "bot", 8080, nil)
	if err != nil {
		t.Fatalf("NewSubmission() error = %v", err)
	}
	if sub.Status != StatusCreated {
		t.Fatalf("status = %s, want %s", sub.Status, StatusCreated)
	}

	if _, err := NewSubmission(contestant, "python", SubmissionTypeSource, "bot", 8080, nil); err == nil {
		t.Fatal("expected unsupported language error")
	}
	if _, err := NewSubmission(contestant, LanguageGo, SubmissionTypeSource, "bot", 80, nil); err == nil || !strings.Contains(err.Error(), "declared_port") {
		t.Fatalf("expected declared_port error, got %v", err)
	}
}

func TestIsTerminalStatus(t *testing.T) {
	if !IsTerminalStatus(StatusBuildFailed) {
		t.Fatal("BUILD_FAILED must be terminal")
	}
	if IsTerminalStatus(StatusReady) {
		t.Fatal("READY must not be terminal")
	}
}
