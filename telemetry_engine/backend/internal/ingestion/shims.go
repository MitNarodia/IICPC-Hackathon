package ingestion

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
)

// track2BotMetricsIn mirrors Track 2's MetricsAggregator::AggregateView plus the
// run/submission/shard context the bot fleet stamps. This shim lets Track 2
// POST its native window straight to us without learning the Envelope format.
type track2BotMetricsIn struct {
	RunID        string  `json:"run_id"`
	SubmissionID string  `json:"submission_id"`
	Source       string  `json:"source"`
	Seq          uint64  `json:"seq"`
	ShardID      uint32  `json:"shard_id"`
	Transactions uint64  `json:"transactions"`
	Errors       uint64  `json:"errors"`
	Timeouts     uint64  `json:"timeouts"`
	Seconds      float64 `json:"seconds"`
	P50US        int64   `json:"p50_us"`
	P90US        int64   `json:"p90_us"`
	P99US        int64   `json:"p99_us"`
	MeanUS       float64 `json:"mean_us"`
	HdrBuckets   []byte  `json:"hdr_buckets,omitempty"`
	WindowStart  int64   `json:"window_start_ts"`
	WindowEnd    int64   `json:"window_end_ts"`
}

// handleTrack2BotMetrics accepts Track 2's native AggregateView JSON and routes
// it as a BotMetrics envelope.
func (s *Server) handleTrack2BotMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var in track2BotMetricsIn
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "invalid bot-metrics JSON", http.StatusBadRequest)
		return
	}
	if in.Source == "" {
		in.Source = "bot-fleet"
	}
	payload := events.BotMetrics{
		ShardID:       in.ShardID,
		Transactions:  in.Transactions,
		Errors:        in.Errors,
		Timeouts:      in.Timeouts,
		WindowSeconds: in.Seconds,
		P50US:         in.P50US,
		P90US:         in.P90US,
		P99US:         in.P99US,
		MeanUS:        in.MeanUS,
		HdrBuckets:    in.HdrBuckets,
		WindowStartTS: in.WindowStart,
		WindowEndTS:   in.WindowEnd,
	}
	env, err := events.NewEnvelope(in.RunID, in.SubmissionID, in.Source, in.Seq, events.TypeBotMetrics, payload)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	if s.route(r.Context(), env) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	} else {
		http.Error(w, "rejected", http.StatusBadRequest)
	}
}

// track1SandboxIn mirrors Track 1's resource/health sample plus run/submission
// context.
type track1SandboxIn struct {
	RunID        string  `json:"run_id"`
	SubmissionID string  `json:"submission_id"`
	Source       string  `json:"source"`
	Seq          uint64  `json:"seq"`
	PodName      string  `json:"pod_name"`
	Namespace    string  `json:"namespace"`
	CPUm         float64 `json:"cpu_millicores"`
	CPULimitm    float64 `json:"cpu_limit_millicores"`
	MemBytes     uint64  `json:"memory_bytes"`
	MemLimit     uint64  `json:"memory_limit_bytes"`
	OpenFDs      uint32  `json:"open_fds"`
	ActiveConns  uint32  `json:"active_connections"`
	Health       string  `json:"health"`
	OOMKilled    bool    `json:"oom_killed"`
	RestartCount uint32  `json:"restart_count"`
	SampleTS     int64   `json:"sample_ts"`
}

// handleTrack1Sandbox accepts Track 1's native sandbox sample and routes it as a
// SandboxMetrics envelope.
func (s *Server) handleTrack1Sandbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var in track1SandboxIn
	if err := json.Unmarshal(body, &in); err != nil {
		http.Error(w, "invalid sandbox JSON", http.StatusBadRequest)
		return
	}
	if in.Source == "" {
		in.Source = "sandbox-runner"
	}
	payload := events.SandboxMetrics{
		PodName:            in.PodName,
		Namespace:          in.Namespace,
		CPUMillicores:      in.CPUm,
		CPULimitMillicores: in.CPULimitm,
		MemoryBytes:        in.MemBytes,
		MemoryLimitBytes:   in.MemLimit,
		OpenFDs:            in.OpenFDs,
		ActiveConnections:  in.ActiveConns,
		Health:             in.Health,
		OOMKilled:          in.OOMKilled,
		RestartCount:       in.RestartCount,
		SampleTS:           in.SampleTS,
	}
	env, err := events.NewEnvelope(in.RunID, in.SubmissionID, in.Source, in.Seq, events.TypeSandboxMetrics, payload)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	if s.route(r.Context(), env) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	} else {
		http.Error(w, "rejected", http.StatusBadRequest)
	}
}
