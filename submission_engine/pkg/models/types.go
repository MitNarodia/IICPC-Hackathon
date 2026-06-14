package models

import "time"

type Language string

const (
	LanguageCPP  Language = "cpp"
	LanguageRust Language = "rust"
	LanguageGo   Language = "go"
)

type SubmissionType string

const (
	SubmissionTypeSource SubmissionType = "source"
	SubmissionTypeBinary SubmissionType = "binary"
)

type SubmissionStatus string

const (
	StatusCreated          SubmissionStatus = "CREATED"
	StatusUploaded         SubmissionStatus = "UPLOADED"
	StatusValidating       SubmissionStatus = "VALIDATING"
	StatusValidated        SubmissionStatus = "VALIDATED"
	StatusBuilding         SubmissionStatus = "BUILDING"
	StatusBuilt            SubmissionStatus = "BUILT"
	StatusScanning         SubmissionStatus = "SCANNING"
	StatusScanned          SubmissionStatus = "SCANNED"
	StatusDeploying        SubmissionStatus = "DEPLOYING"
	StatusHealthCheck      SubmissionStatus = "HEALTH_CHECK"
	StatusReady            SubmissionStatus = "READY"
	StatusDegraded         SubmissionStatus = "DEGRADED"
	StatusTerminated       SubmissionStatus = "TERMINATED"
	StatusUploadFailed     SubmissionStatus = "UPLOAD_FAILED"
	StatusValidationFailed SubmissionStatus = "VALIDATION_FAILED"
	StatusBuildFailed      SubmissionStatus = "BUILD_FAILED"
	StatusScanFailed       SubmissionStatus = "SCAN_FAILED"
	StatusDeployFailed     SubmissionStatus = "DEPLOY_FAILED"
	StatusHealthFailed     SubmissionStatus = "HEALTH_FAILED"
)

type BuildStatus string

const (
	BuildPending BuildStatus = "PENDING"
	BuildRunning BuildStatus = "RUNNING"
	BuildSuccess BuildStatus = "SUCCESS"
	BuildFailed  BuildStatus = "FAILED"
)

type ScanStatus string

const (
	ScanPending ScanStatus = "PENDING"
	ScanRunning ScanStatus = "RUNNING"
	ScanPassed  ScanStatus = "PASSED"
	ScanFailed  ScanStatus = "FAILED"
)

type DeploymentStatus string

const (
	DeploymentPending    DeploymentStatus = "PENDING"
	DeploymentScheduling DeploymentStatus = "SCHEDULING"
	DeploymentRunning    DeploymentStatus = "RUNNING"
	DeploymentReady      DeploymentStatus = "READY"
	DeploymentDegraded   DeploymentStatus = "DEGRADED"
	DeploymentTerminated DeploymentStatus = "TERMINATED"
	DeploymentFailed     DeploymentStatus = "FAILED"
)

type EndpointProtocol string

const (
	ProtocolHTTP EndpointProtocol = "http"
	ProtocolWS   EndpointProtocol = "ws"
	ProtocolGRPC EndpointProtocol = "grpc"
)

type EndpointStatus string

const (
	EndpointActive   EndpointStatus = "ACTIVE"
	EndpointInactive EndpointStatus = "INACTIVE"
)

type Submission struct {
	ID             string                 `json:"id"`
	ContestantID   string                 `json:"contestant_id"`
	Language       Language               `json:"language"`
	Type           SubmissionType         `json:"submission_type"`
	Status         SubmissionStatus       `json:"status"`
	Entrypoint     string                 `json:"entrypoint"`
	DeclaredPort   int                    `json:"declared_port"`
	ArtifactURI    string                 `json:"artifact_uri,omitempty"`
	ArtifactSHA256 string                 `json:"artifact_sha256,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type Build struct {
	ID           string      `json:"id"`
	SubmissionID string      `json:"submission_id"`
	Status       BuildStatus `json:"status"`
	ImageRef     string      `json:"image_ref,omitempty"`
	ImageDigest  string      `json:"image_digest,omitempty"`
	LogsURI      string      `json:"build_logs_uri,omitempty"`
	StartedAt    *time.Time  `json:"started_at,omitempty"`
	FinishedAt   *time.Time  `json:"finished_at,omitempty"`
	Error        string      `json:"error,omitempty"`
}

type Scan struct {
	ID           string                 `json:"id"`
	SubmissionID string                 `json:"submission_id"`
	ImageDigest  string                 `json:"image_digest"`
	Status       ScanStatus             `json:"status"`
	Findings     map[string]interface{} `json:"findings,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

type Deployment struct {
	ID           string           `json:"id"`
	SubmissionID string           `json:"submission_id"`
	Status       DeploymentStatus `json:"status"`
	PodName      string           `json:"pod_name,omitempty"`
	Namespace    string           `json:"namespace"`
	NodeName     string           `json:"node_name,omitempty"`
	CPUCores     int              `json:"cpu_cores"`
	MemoryMB     int              `json:"memory_mb"`
	RuntimeClass string           `json:"runtime_class"`
	CreatedAt    time.Time        `json:"created_at"`
	TerminatedAt *time.Time       `json:"terminated_at,omitempty"`
}

type Endpoint struct {
	ID           string           `json:"id"`
	SubmissionID string           `json:"submission_id"`
	DeploymentID string           `json:"deployment_id"`
	InternalURL  string           `json:"internal_url"`
	ServiceName  string           `json:"service_name"`
	Protocol     EndpointProtocol `json:"protocol"`
	Status       EndpointStatus   `json:"status"`
	RegisteredAt time.Time        `json:"registered_at"`
}

type HealthSample struct {
	Time         time.Time `json:"time"`
	SubmissionID string    `json:"submission_id"`
	DeploymentID string    `json:"deployment_id"`
	Healthy      bool      `json:"healthy"`
	LatencyMS    float64   `json:"latency_ms"`
	CPUPct       float64   `json:"cpu_pct"`
	MemMB        float64   `json:"mem_mb"`
	Restarts     int       `json:"restarts"`
}

type AuditLog struct {
	ID           string                 `json:"id"`
	SubmissionID string                 `json:"submission_id,omitempty"`
	Actor        string                 `json:"actor"`
	Action       string                 `json:"action"`
	PrevState    SubmissionStatus       `json:"prev_state,omitempty"`
	NewState     SubmissionStatus       `json:"new_state,omitempty"`
	Detail       map[string]interface{} `json:"detail,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}
