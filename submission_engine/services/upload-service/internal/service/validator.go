package service

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/iicpc/track1/submission-engine/pkg/models"
)

type Limits struct {
	MaxUploadBytes       int64
	MaxDecompressedBytes int64
	MaxFiles             int
}

type ArtifactMetadata struct {
	Language       models.Language
	SubmissionType models.SubmissionType
	ExpectedSHA256 string
}

type ValidationResult struct {
	SHA256       string
	Files        int
	Uncompressed int64
}

func DefaultLimits() Limits {
	return Limits{MaxUploadBytes: 104857600, MaxDecompressedBytes: 536870912, MaxFiles: 10000}
}

func ValidateArtifact(filePath string, meta ArtifactMetadata, limits Limits) (ValidationResult, error) {
	if limits.MaxUploadBytes == 0 {
		limits = DefaultLimits()
	}
	if err := models.ValidateLanguage(meta.Language); err != nil {
		return ValidationResult{}, err
	}
	if err := models.ValidateSubmissionType(meta.SubmissionType); err != nil {
		return ValidationResult{}, err
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return ValidationResult{}, err
	}
	if info.Size() > limits.MaxUploadBytes {
		return ValidationResult{}, fmt.Errorf("artifact exceeds max upload size")
	}
	digest, err := sha256File(filePath)
	if err != nil {
		return ValidationResult{}, err
	}
	if meta.ExpectedSHA256 != "" && !strings.EqualFold(meta.ExpectedSHA256, digest) {
		return ValidationResult{}, errors.New("sha256 mismatch")
	}
	if meta.SubmissionType == models.SubmissionTypeBinary {
		if err := validateELF(filePath); err != nil {
			return ValidationResult{}, err
		}
		return ValidationResult{SHA256: digest, Files: 1, Uncompressed: info.Size()}, nil
	}
	result, err := validateSourceTarGz(filePath, meta.Language, limits)
	if err != nil {
		return ValidationResult{}, err
	}
	result.SHA256 = digest
	return result, nil
}

func validateELF(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return err
	}
	if string(magic[:]) != "\x7fELF" {
		return errors.New("binary artifact is not an ELF executable")
	}
	return nil
}

func validateSourceTarGz(filePath string, language models.Language, limits Limits) (ValidationResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return ValidationResult{}, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return ValidationResult{}, errors.New("source artifact must be a gzip-compressed tar archive")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var files int
	var total int64
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ValidationResult{}, err
		}
		files++
		if files > limits.MaxFiles {
			return ValidationResult{}, errors.New("archive contains too many files")
		}
		clean, err := validateArchivePath(header.Name)
		if err != nil {
			return ValidationResult{}, err
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			if _, err := validateArchivePath(header.Linkname); err != nil {
				return ValidationResult{}, fmt.Errorf("unsafe link target %q", header.Linkname)
			}
		}
		if header.Mode&04000 != 0 {
			return ValidationResult{}, fmt.Errorf("setuid bit is forbidden on %s", clean)
		}
		if header.Typeflag == tar.TypeReg {
			total += header.Size
			if total > limits.MaxDecompressedBytes {
				return ValidationResult{}, errors.New("archive exceeds decompressed size cap")
			}
			seen[path.Base(clean)] = true
			seen[clean] = true
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return ValidationResult{}, err
			}
		}
	}
	if !hasLanguageManifest(language, seen) {
		return ValidationResult{}, fmt.Errorf("source archive missing required %s manifest or main file", language)
	}
	return ValidationResult{Files: files, Uncompressed: total}, nil
}

func validateArchivePath(name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return clean, nil
}

func hasLanguageManifest(language models.Language, seen map[string]bool) bool {
	switch language {
	case models.LanguageCPP:
		return seen["CMakeLists.txt"] || seen["Makefile"] || seen["main.cpp"] || seen["main.cc"] || seen["main.cxx"]
	case models.LanguageRust:
		return seen["Cargo.toml"]
	case models.LanguageGo:
		return seen["go.mod"] || seen["main.go"]
	default:
		return false
	}
}

func sha256File(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
