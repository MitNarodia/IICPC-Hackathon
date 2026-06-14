package service

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/iicpc/track1/submission-engine/pkg/models"
)

type ProbeResult struct {
	Healthy   bool
	LatencyMS float64
	Error     string
}

type Prober struct {
	Timeout time.Duration
}

func (p Prober) ProbeHTTP(ctx context.Context, url string) ProbeResult {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{Healthy: false, Error: err.Error()}
	}
	client := http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	latency := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		return ProbeResult{Healthy: false, LatencyMS: latency, Error: err.Error()}
	}
	defer resp.Body.Close()
	return ProbeResult{Healthy: resp.StatusCode >= 200 && resp.StatusCode < 400, LatencyMS: latency}
}

func (p Prober) ProbeTCP(ctx context.Context, address string) ProbeResult {
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", address)
	latency := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		return ProbeResult{Healthy: false, LatencyMS: latency, Error: err.Error()}
	}
	_ = conn.Close()
	return ProbeResult{Healthy: true, LatencyMS: latency}
}

func (r ProbeResult) ToSample(submissionID, deploymentID string) models.HealthSample {
	return models.HealthSample{
		Time:         time.Now().UTC(),
		SubmissionID: submissionID,
		DeploymentID: deploymentID,
		Healthy:      r.Healthy,
		LatencyMS:    r.LatencyMS,
	}
}
