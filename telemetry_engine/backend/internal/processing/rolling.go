package processing

import (
	"math"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/models"
)

// RollingStats tracks the whole-run, continuously-updated view for ONE
// submission: cumulative totals, peak/current TPS, and — crucially for the
// STABILITY score — the running mean and variance of per-window TPS via
// Welford's online algorithm (numerically stable, O(1) per update, no need to
// store the full TPS history).
//
// Why coefficient of variation (CoV = stddev/mean)? A fast engine that
// oscillates wildly between 200k and 20k TPS is worse, operationally, than a
// steady 100k engine — bursty tails blow latency SLAs. CoV is scale-free so it
// compares submissions of very different throughput fairly.
type RollingStats struct {
	runID, submissionID string

	totalTxns   uint64
	totalErrors uint64
	peakTPS     float64
	currentTPS  float64

	p50, p90, p99 int64
	errorRate     float64

	// Welford accumulators over the sequence of window TPS values.
	n       int64
	mean    float64
	m2      float64 // sum of squares of differences from the current mean
}

// NewRollingStats creates an empty tracker for a (run, submission).
func NewRollingStats(runID, submissionID string) *RollingStats {
	return &RollingStats{runID: runID, submissionID: submissionID}
}

// ObserveWindow folds a freshly-closed window into the rolling view. Call this
// once per tumbling window so the TPS variance reflects real, non-overlapping
// intervals (overlapping sliding windows would correlate samples and understate
// variance).
func (rs *RollingStats) ObserveWindow(a Aggregate) {
	rs.totalTxns += a.Transactions
	rs.totalErrors += a.Errors
	rs.currentTPS = a.TPS
	if a.TPS > rs.peakTPS {
		rs.peakTPS = a.TPS
	}
	rs.p50, rs.p90, rs.p99 = a.P50US, a.P90US, a.P99US
	rs.errorRate = a.ErrorRate

	// Welford online update.
	rs.n++
	delta := a.TPS - rs.mean
	rs.mean += delta / float64(rs.n)
	delta2 := a.TPS - rs.mean
	rs.m2 += delta * delta2
}

// stdDev returns the sample standard deviation of per-window TPS.
func (rs *RollingStats) stdDev() float64 {
	if rs.n < 2 {
		return 0
	}
	return math.Sqrt(rs.m2 / float64(rs.n-1))
}

// cov returns the coefficient of variation (stddev / mean), 0 when mean is 0.
func (rs *RollingStats) cov() float64 {
	if rs.mean == 0 {
		return 0
	}
	return rs.stdDev() / rs.mean
}

// Snapshot renders the current rolling view as a transferable model.
func (rs *RollingStats) Snapshot() models.RollingStats {
	return models.RollingStats{
		RunID:             rs.runID,
		SubmissionID:      rs.submissionID,
		UpdatedAt:         time.Now().UTC(),
		TotalTransactions: rs.totalTxns,
		TotalErrors:       rs.totalErrors,
		PeakTPS:           rs.peakTPS,
		CurrentTPS:        rs.currentTPS,
		P50US:             rs.p50,
		P90US:             rs.p90,
		P99US:             rs.p99,
		ErrorRate:         rs.errorRate,
		TPSStdDev:         rs.stdDev(),
		TPSCoV:            rs.cov(),
	}
}
