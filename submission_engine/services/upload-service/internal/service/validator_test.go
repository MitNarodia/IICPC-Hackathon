package service

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iicpc/track1/submission-engine/pkg/models"
)

func TestValidateSourceTarGz(t *testing.T) {
	archive := writeTarGz(t, map[string]string{
		"bot/go.mod":  "module bot\n",
		"bot/main.go": "package main\nfunc main(){}\n",
	})
	result, err := ValidateArtifact(archive, ArtifactMetadata{
		Language:       models.LanguageGo,
		SubmissionType: models.SubmissionTypeSource,
	}, Limits{MaxUploadBytes: 1 << 20, MaxDecompressedBytes: 1 << 20, MaxFiles: 10})
	if err != nil {
		t.Fatalf("ValidateArtifact() error = %v", err)
	}
	if result.Files != 2 || result.SHA256 == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRejectPathTraversal(t *testing.T) {
	archive := writeTarGz(t, map[string]string{"../go.mod": "module bad\n"})
	_, err := ValidateArtifact(archive, ArtifactMetadata{
		Language:       models.LanguageGo,
		SubmissionType: models.SubmissionTypeSource,
	}, Limits{MaxUploadBytes: 1 << 20, MaxDecompressedBytes: 1 << 20, MaxFiles: 10})
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestValidateELF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot")
	if err := os.WriteFile(path, []byte("\x7fELFrest"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateArtifact(path, ArtifactMetadata{
		Language:       models.LanguageCPP,
		SubmissionType: models.SubmissionTypeBinary,
	}, DefaultLimits()); err != nil {
		t.Fatalf("ValidateArtifact() error = %v", err)
	}
}

func writeTarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "artifact.tar.gz")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}
