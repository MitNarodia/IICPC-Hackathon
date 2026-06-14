package events

type SubmissionCreatedData struct {
	ContestantID   string `json:"contestant_id"`
	Language       string `json:"language"`
	SubmissionType string `json:"submission_type"`
	UploadURL      string `json:"upload_url"`
}

type SubmissionUploadedData struct {
	ArtifactURI    string `json:"artifact_uri"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	DeclaredPort   int    `json:"declared_port"`
	Entrypoint     string `json:"entrypoint"`
}

type ValidationFailedData struct {
	Reason string `json:"reason"`
}

type BuildRequestedData struct {
	ArtifactURI string `json:"artifact_uri"`
	Language    string `json:"language"`
}

type BuildSucceededData struct {
	ImageRef    string `json:"image_ref"`
	ImageDigest string `json:"image_digest"`
}

type BuildFailedData struct {
	Error   string `json:"error"`
	LogsURI string `json:"logs_uri"`
}

type ScanPassedData struct {
	ImageDigest     string                 `json:"image_digest"`
	FindingsSummary map[string]interface{} `json:"findings_summary"`
}

type ScanFailedData struct {
	FindingsSummary map[string]interface{} `json:"findings_summary"`
}

type DeploymentRequestedData struct {
	ImageRef     string `json:"image_ref"`
	CPUCores     int    `json:"cpu_cores"`
	MemoryMB     int    `json:"memory_mb"`
	DeclaredPort int    `json:"declared_port"`
}

type DeploymentReadyData struct {
	PodName     string `json:"pod_name"`
	InternalURL string `json:"internal_url"`
	ServiceName string `json:"service_name"`
}

type DeploymentFailedData struct {
	Reason string `json:"reason"`
}

type HealthReadyData struct {
	InternalURL string `json:"internal_url"`
}

type HealthDegradedData struct {
	Reason        string  `json:"reason"`
	LastLatencyMS float64 `json:"last_latency_ms"`
}

type TeardownRequestedData struct {
	Reason string `json:"reason"`
}
