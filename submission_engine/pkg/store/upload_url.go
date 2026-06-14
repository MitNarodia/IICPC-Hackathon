package store

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type StaticUploadURLProvider struct {
	BaseURL string
}

func (p StaticUploadURLProvider) PresignUploadURL(_ context.Context, submissionID string) (string, error) {
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = "s3://raw-uploads"
	}
	if strings.HasPrefix(base, "s3://") {
		return fmt.Sprintf("%s/%s/artifact", base, submissionID), nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + submissionID + "/artifact"
	return u.String(), nil
}
