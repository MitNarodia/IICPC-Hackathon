package profiles

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iicpc/track1/submission-engine/pkg/models"
)

type Profile struct {
	Language     models.Language
	BuilderBase  string
	RuntimeBase  string
	BuildCommand string
	OutputPath   string
}

var Defaults = map[models.Language]Profile{
	models.LanguageCPP: {
		Language:     models.LanguageCPP,
		BuilderBase:  "gcc:13-bookworm",
		RuntimeBase:  "gcr.io/distroless/cc-debian12:nonroot",
		BuildCommand: "if [ -f Makefile ]; then make; else g++ -O2 -std=c++20 -static -o /out/bot main.cpp; fi",
		OutputPath:   "/out/bot",
	},
	models.LanguageRust: {
		Language:     models.LanguageRust,
		BuilderBase:  "rust:1.79-bookworm",
		RuntimeBase:  "gcr.io/distroless/cc-debian12:nonroot",
		BuildCommand: "cargo build --release && cp target/release/* /out/bot",
		OutputPath:   "/out/bot",
	},
	models.LanguageGo: {
		Language:     models.LanguageGo,
		BuilderBase:  "golang:1.22-alpine",
		RuntimeBase:  "gcr.io/distroless/static-debian12:nonroot",
		BuildCommand: "CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/bot ./...",
		OutputPath:   "/out/bot",
	},
}

type RenderRequest struct {
	Language       models.Language
	SubmissionType models.SubmissionType
	Entrypoint     string
	DeclaredPort   int
}

func RenderDockerfile(req RenderRequest) (string, error) {
	if err := models.ValidateLanguage(req.Language); err != nil {
		return "", err
	}
	if err := models.ValidateSubmissionType(req.SubmissionType); err != nil {
		return "", err
	}
	if err := models.ValidateDeclaredPort(req.DeclaredPort); err != nil {
		return "", err
	}
	profile := Defaults[req.Language]
	if req.SubmissionType == models.SubmissionTypeBinary {
		return renderBinaryDockerfile(profile, req)
	}
	return renderSourceDockerfile(profile, req)
}

func renderSourceDockerfile(profile Profile, req RenderRequest) (string, error) {
	cmd, err := jsonCommand(req.Entrypoint)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`# syntax=docker/dockerfile:1.7
FROM %s AS build
WORKDIR /src
COPY . .
RUN mkdir -p /out && %s

FROM %s
USER 65532:65532
WORKDIR /app
COPY --from=build --chown=65532:65532 %s /app/bot
EXPOSE %d
ENTRYPOINT %s
`, profile.BuilderBase, profile.BuildCommand, profile.RuntimeBase, profile.OutputPath, req.DeclaredPort, cmd), nil
}

func renderBinaryDockerfile(profile Profile, req RenderRequest) (string, error) {
	cmd, err := jsonCommand(req.Entrypoint)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`# syntax=docker/dockerfile:1.7
FROM %s
USER 65532:65532
WORKDIR /app
COPY --chown=65532:65532 ./bot /app/bot
EXPOSE %d
ENTRYPOINT %s
`, profile.RuntimeBase, req.DeclaredPort, cmd), nil
}

func jsonCommand(entrypoint string) (string, error) {
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint == "" {
		entrypoint = "/app/bot"
	}
	parts := strings.Fields(entrypoint)
	payload, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
