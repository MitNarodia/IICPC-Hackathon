package service

import (
	"fmt"

	"github.com/iicpc/track1/submission-engine/pkg/security"
	"github.com/iicpc/track1/submission-engine/services/build-service/internal/profiles"
)

type BuildPlan struct {
	Dockerfile string
	Job        map[string]interface{}
	LogsURI    string
}

type Planner struct {
	Namespace string
}

func (p Planner) Plan(submissionID string, req profiles.RenderRequest) (BuildPlan, error) {
	dockerfile, err := profiles.RenderDockerfile(req)
	if err != nil {
		return BuildPlan{}, err
	}
	job := security.BuildRootlessJob(security.BuildJobRequest{
		Name:      "build-" + submissionID,
		Namespace: p.Namespace,
	})
	return BuildPlan{
		Dockerfile: dockerfile,
		Job:        job,
		LogsURI:    fmt.Sprintf("s3://build-logs/%s", submissionID),
	}, nil
}
