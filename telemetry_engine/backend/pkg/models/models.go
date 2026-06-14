// Package models defines the DERIVED domain types Track 3 produces — as opposed
// to package events, which defines the RAW telemetry Track 3 consumes. These
// types flow on the analytics.* topics and persist to PostgreSQL/TimescaleDB.
package models

import "time"

// WindowAggregate is the stream-processor's output for one (run, submission)
// over one time window. It is the unit the scoring engine consumes and the
// unit the dashboard charts. All latencies are microseconds.
type WindowAggregate struct {
	RunID        string    `json:"run_id"`
	SubmissionID string    `json:"submission_id"`
	WindowStart  time.Time `json:"window_start"`
	WindowEnd    time.Time `json:"window_end"`
	WindowKind   string    `json:"window_kind"` // "tumbling" | "sliding" | "rolling"

	// Throughput
	Transactions uint64  `json:"transactions"`
	Errors       uint64  `json:"errors"`
	Timeouts     uint64  `json:"timeouts"`
	TPS          float64 `json:"tps"`
	ErrorRate    float64 `json:"error_rate"` // errors / (transactions+errors), 0..1

	// Latency percentiles (microseconds)
	P50US  int64   `json:"p50_us"`
	P90US  int64   `json:"p90_us"`
	P99US  int64   `json:"p99_us"`
	MaxUS  int64   `json:"max_us"`
	MeanUS float64 `json:"mean_us"`

	// Sample count behind the percentiles (confidence signal).
	SampleCount int64 `json:"sample_count"`
}

// RollingStats is the continuously-updated, whole-run view for a submission.
// Unlike a window, it never resets; it is the authoritative end-of-run number.
type RollingStats struct {
	RunID        string    `json:"run_id"`
	SubmissionID string    `json:"submission_id"`
	UpdatedAt    time.Time `json:"updated_at"`

	TotalTransactions uint64  `json:"total_transactions"`
	TotalErrors       uint64  `json:"total_errors"`
	PeakTPS           float64 `json:"peak_tps"`     // max sustained window TPS
	CurrentTPS        float64 `json:"current_tps"`  // latest window TPS
	P50US             int64   `json:"p50_us"`
	P90US             int64   `json:"p90_us"`
	P99US             int64   `json:"p99_us"`
	ErrorRate         float64 `json:"error_rate"`

	// Stability: coefficient of variation of per-window TPS (lower is steadier).
	TPSStdDev float64 `json:"tps_stddev"`
	TPSCoV    float64 `json:"tps_cov"`
}

// ValidationResult is the validation-engine's verdict for a (run, submission).
// It is cumulative: each new finding decrements the correctness signal.
type ValidationResult struct {
	RunID        string    `json:"run_id"`
	SubmissionID string    `json:"submission_id"`
	UpdatedAt    time.Time `json:"updated_at"`

	OrdersChecked  uint64 `json:"orders_checked"`
	TradesChecked  uint64 `json:"trades_checked"`
	Violations     uint64 `json:"violations"`
	ViolationsByRule map[string]uint64 `json:"violations_by_rule"`

	// CorrectnessScore in [0,1]: 1 - (violations / max(1, checks)), clamped.
	CorrectnessScore float64 `json:"correctness_score"`

	// Most recent few human-readable findings, for the contestant detail panel.
	RecentFindings []Finding `json:"recent_findings,omitempty"`
}

// Finding is a single correctness violation with enough context to explain it.
type Finding struct {
	Rule    string    `json:"rule"`     // e.g. "price_time_priority"
	Message string    `json:"message"`  // human-readable explanation
	OrderID uint64    `json:"order_id"`
	At      time.Time `json:"at"`
}

// Score is the scoring-engine's composite output. It is what ranks the board.
type Score struct {
	RunID        string    `json:"run_id"`
	SubmissionID string    `json:"submission_id"`
	ContestantID string    `json:"contestant_id,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
	ComputedAt   time.Time `json:"computed_at"`

	// Component sub-scores, each normalized to [0,100].
	LatencyScore     float64 `json:"latency_score"`
	ThroughputScore  float64 `json:"throughput_score"`
	CorrectnessScore float64 `json:"correctness_score"`
	StabilityScore   float64 `json:"stability_score"`

	// Composite, weighted sum of the four, [0,100].
	Composite float64 `json:"composite"`

	// Snapshot of the raw metrics that produced this score (for the UI).
	TPS       float64 `json:"tps"`
	P50US     int64   `json:"p50_us"`
	P99US     int64   `json:"p99_us"`
	ErrorRate float64 `json:"error_rate"`
}

// LeaderboardEntry is the fully-denormalized row the frontend renders. It is
// the union of Score + rank + live deltas, stored in Redis and pushed over WS.
type LeaderboardEntry struct {
	Rank         int     `json:"rank"`
	PrevRank     int     `json:"prev_rank"` // for up/down arrows in the UI
	RunID        string  `json:"run_id"`
	SubmissionID string  `json:"submission_id"`
	DisplayName  string  `json:"display_name"`
	Composite    float64 `json:"composite"`

	LatencyScore     float64 `json:"latency_score"`
	ThroughputScore  float64 `json:"throughput_score"`
	CorrectnessScore float64 `json:"correctness_score"`
	StabilityScore   float64 `json:"stability_score"`

	TPS       float64 `json:"tps"`
	P50US     int64   `json:"p50_us"`
	P99US     int64   `json:"p99_us"`
	ErrorRate float64 `json:"error_rate"`
	Health    string  `json:"health"` // from sandbox metrics: READY/DEGRADED/UNHEALTHY

	UpdatedAt time.Time `json:"updated_at"`
}
