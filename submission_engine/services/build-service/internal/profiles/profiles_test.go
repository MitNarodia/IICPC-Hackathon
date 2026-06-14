package profiles

import (
	"strings"
	"testing"

	"github.com/iicpc/track1/submission-engine/pkg/models"
)

func TestRenderDockerfileForAllLanguages(t *testing.T) {
	for _, language := range []models.Language{models.LanguageCPP, models.LanguageRust, models.LanguageGo} {
		dockerfile, err := RenderDockerfile(RenderRequest{
			Language:       language,
			SubmissionType: models.SubmissionTypeSource,
			Entrypoint:     "/app/bot",
			DeclaredPort:   8080,
		})
		if err != nil {
			t.Fatalf("%s RenderDockerfile() error = %v", language, err)
		}
		if !strings.Contains(dockerfile, "@sha256:") {
			t.Fatalf("%s Dockerfile must use pinned digest bases:\n%s", language, dockerfile)
		}
		if !strings.Contains(dockerfile, "USER 65532:65532") {
			t.Fatalf("%s Dockerfile must run non-root:\n%s", language, dockerfile)
		}
	}
}

func TestRenderDockerfileRejectsBadPort(t *testing.T) {
	_, err := RenderDockerfile(RenderRequest{
		Language:       models.LanguageGo,
		SubmissionType: models.SubmissionTypeSource,
		DeclaredPort:   80,
	})
	if err == nil {
		t.Fatal("expected bad port error")
	}
}

func TestBinaryDockerfileHasNoBuilderStage(t *testing.T) {
	dockerfile, err := RenderDockerfile(RenderRequest{
		Language:       models.LanguageGo,
		SubmissionType: models.SubmissionTypeBinary,
		Entrypoint:     "/app/bot",
		DeclaredPort:   8080,
	})
	if err != nil {
		t.Fatalf("RenderDockerfile() error = %v", err)
	}
	if strings.Contains(dockerfile, " AS build") {
		t.Fatalf("binary Dockerfile should wrap without build stage:\n%s", dockerfile)
	}
}
