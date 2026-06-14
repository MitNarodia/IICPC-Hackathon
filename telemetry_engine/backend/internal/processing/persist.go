package processing

import (
	"context"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	"github.com/iicpc/track3/telemetry-engine/pkg/models"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
)

// persistWindow upserts a window aggregate into the TimescaleDB hypertable.
// Only tumbling windows are persisted durably (they are non-overlapping and
// replayable); sliding windows are transient and live only on the bus + Redis.
func (p *Processor) persistWindow(ctx context.Context, wa models.WindowAggregate) {
	if p.pg == nil || wa.WindowKind != "tumbling" {
		return
	}
	const q = `
INSERT INTO window_aggregates
  (run_id, submission_id, window_start, window_end, window_kind,
   transactions, errors, timeouts, tps, error_rate,
   p50_us, p90_us, p99_us, max_us, mean_us, sample_count)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (run_id, submission_id, window_start, window_kind)
DO UPDATE SET
  transactions = EXCLUDED.transactions,
  errors       = EXCLUDED.errors,
  timeouts     = EXCLUDED.timeouts,
  tps          = EXCLUDED.tps,
  error_rate   = EXCLUDED.error_rate,
  p50_us       = EXCLUDED.p50_us,
  p90_us       = EXCLUDED.p90_us,
  p99_us       = EXCLUDED.p99_us,
  max_us       = EXCLUDED.max_us,
  mean_us      = EXCLUDED.mean_us,
  sample_count = EXCLUDED.sample_count;`
	if _, err := p.pg.Pool.Exec(ctx, q,
		wa.RunID, wa.SubmissionID, wa.WindowStart, wa.WindowEnd, wa.WindowKind,
		wa.Transactions, wa.Errors, wa.Timeouts, wa.TPS, wa.ErrorRate,
		wa.P50US, wa.P90US, wa.P99US, wa.MaxUS, wa.MeanUS, wa.SampleCount,
	); err != nil {
		p.log.Error("persist window", logpkg.F("err", err.Error()))
	}
}

// persistRolling upserts the whole-run rolling snapshot for a submission.
func (p *Processor) persistRolling(ctx context.Context, rs models.RollingStats) {
	if p.pg == nil {
		return
	}
	const q = `
INSERT INTO rolling_stats
  (run_id, submission_id, updated_at, total_transactions, total_errors,
   peak_tps, current_tps, p50_us, p90_us, p99_us, error_rate,
   tps_stddev, tps_cov)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT (run_id, submission_id)
DO UPDATE SET
  updated_at         = EXCLUDED.updated_at,
  total_transactions = EXCLUDED.total_transactions,
  total_errors       = EXCLUDED.total_errors,
  peak_tps           = GREATEST(rolling_stats.peak_tps, EXCLUDED.peak_tps),
  current_tps        = EXCLUDED.current_tps,
  p50_us             = EXCLUDED.p50_us,
  p90_us             = EXCLUDED.p90_us,
  p99_us             = EXCLUDED.p99_us,
  error_rate         = EXCLUDED.error_rate,
  tps_stddev         = EXCLUDED.tps_stddev,
  tps_cov            = EXCLUDED.tps_cov;`
	if _, err := p.pg.Pool.Exec(ctx, q,
		rs.RunID, rs.SubmissionID, rs.UpdatedAt, rs.TotalTransactions, rs.TotalErrors,
		rs.PeakTPS, rs.CurrentTPS, rs.P50US, rs.P90US, rs.P99US, rs.ErrorRate,
		rs.TPSStdDev, rs.TPSCoV,
	); err != nil {
		p.log.Error("persist rolling", logpkg.F("err", err.Error()))
	}
}

// persistSandboxSample inserts a Track 1 sandbox health sample into the
// sandbox_samples hypertable so dashboard history queries and health status
// are backed by durable storage.
func (p *Processor) persistSandboxSample(ctx context.Context, runID, submissionID string, sm events.SandboxMetrics) {
	if p.pg == nil {
		p.log.Warn("sandbox_metrics: postgres unavailable, skipping persist", logpkg.F(
			"run_id", runID, "submission_id", submissionID))
		return
	}

	sampleTime := time.Unix(0, sm.SampleTS)
	if sm.SampleTS == 0 {
		sampleTime = time.Now().UTC()
	}

	const q = `
INSERT INTO sandbox_samples
  (run_id, submission_id, sample_ts, pod_name, namespace,
   cpu_millicores, cpu_limit_millicores, memory_bytes, memory_limit_bytes,
   open_fds, active_connections, health, oom_killed, restart_count)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
	if _, err := p.pg.Pool.Exec(ctx, q,
		runID, submissionID, sampleTime, sm.PodName, sm.Namespace,
		sm.CPUMillicores, sm.CPULimitMillicores, sm.MemoryBytes, sm.MemoryLimitBytes,
		sm.OpenFDs, sm.ActiveConnections, sm.Health, sm.OOMKilled, sm.RestartCount,
	); err != nil {
		p.log.Error("persist sandbox_sample", logpkg.F("err", err.Error()))
	} else {
		p.log.Info("sandbox_sample persisted", logpkg.F(
			"run_id", runID, "submission_id", submissionID,
			"health", sm.Health, "sample_ts", sampleTime.Format(time.RFC3339)))
	}
}
