package models

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidateLanguage(language Language) error {
	switch language {
	case LanguageCPP, LanguageRust, LanguageGo:
		return nil
	default:
		return fmt.Errorf("unsupported language %q", language)
	}
}

func ValidateSubmissionType(t SubmissionType) error {
	switch t {
	case SubmissionTypeSource, SubmissionTypeBinary:
		return nil
	default:
		return fmt.Errorf("unsupported submission_type %q", t)
	}
}

func ValidateDeclaredPort(port int) error {
	if port < 1024 || port > 65535 {
		return fmt.Errorf("declared_port must be in [1024,65535], got %d", port)
	}
	return nil
}

func ValidateEntrypoint(entrypoint string) error {
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint == "" {
		return errors.New("entrypoint is required")
	}
	if strings.Contains(entrypoint, "\x00") || strings.Contains(entrypoint, "\n") {
		return errors.New("entrypoint contains invalid control characters")
	}
	return nil
}

func ValidateUUID(id string) error {
	if !uuidPattern.MatchString(id) {
		return fmt.Errorf("invalid uuid %q", id)
	}
	return nil
}

func IsTerminalStatus(status SubmissionStatus) bool {
	switch status {
	case StatusTerminated, StatusUploadFailed, StatusValidationFailed, StatusBuildFailed, StatusScanFailed, StatusDeployFailed, StatusHealthFailed:
		return true
	default:
		return false
	}
}

func NewSubmission(contestantID string, language Language, submissionType SubmissionType, entrypoint string, declaredPort int, metadata map[string]interface{}) (*Submission, error) {
	if err := ValidateUUID(contestantID); err != nil {
		return nil, err
	}
	if err := ValidateLanguage(language); err != nil {
		return nil, err
	}
	if err := ValidateSubmissionType(submissionType); err != nil {
		return nil, err
	}
	if err := ValidateEntrypoint(entrypoint); err != nil {
		return nil, err
	}
	if err := ValidateDeclaredPort(declaredPort); err != nil {
		return nil, err
	}
	id, err := NewUUIDv7()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return &Submission{
		ID:           id,
		ContestantID: contestantID,
		Language:     language,
		Type:         submissionType,
		Status:       StatusCreated,
		Entrypoint:   strings.TrimSpace(entrypoint),
		DeclaredPort: declaredPort,
		CreatedAt:    now,
		UpdatedAt:    now,
		Metadata:     metadata,
	}, nil
}
