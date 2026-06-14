package processing

import (
	"math"
	"testing"
)

// A perfectly steady engine has zero TPS variance and therefore zero CoV.
func TestRollingStatsSteadyHasZeroCoV(t *testing.T) {
	rs := NewRollingStats("run", "sub")
	for i := 0; i < 5; i++ {
		rs.ObserveWindow(Aggregate{TPS: 100, Transactions: 100})
	}
	snap := rs.Snapshot()
	if snap.TPSStdDev != 0 {
		t.Errorf("stddev = %.4f, want 0", snap.TPSStdDev)
	}
	if snap.TPSCoV != 0 {
		t.Errorf("cov = %.4f, want 0", snap.TPSCoV)
	}
	if snap.TotalTransactions != 500 {
		t.Errorf("total txns = %d, want 500", snap.TotalTransactions)
	}
}

// Welford must reproduce the textbook sample stddev / CoV.
func TestRollingStatsVarianceMatchesClosedForm(t *testing.T) {
	rs := NewRollingStats("run", "sub")
	for _, tps := range []float64{100, 200, 300} {
		rs.ObserveWindow(Aggregate{TPS: tps, Transactions: uint64(tps)})
	}
	snap := rs.Snapshot()
	// mean=200, sample var=10000, stddev=100, cov=0.5.
	if math.Abs(snap.TPSStdDev-100) > 1e-6 {
		t.Errorf("stddev = %.6f, want 100", snap.TPSStdDev)
	}
	if math.Abs(snap.TPSCoV-0.5) > 1e-6 {
		t.Errorf("cov = %.6f, want 0.5", snap.TPSCoV)
	}
	if snap.PeakTPS != 300 {
		t.Errorf("peak tps = %.1f, want 300", snap.PeakTPS)
	}
	if snap.CurrentTPS != 300 {
		t.Errorf("current tps = %.1f, want 300 (last window)", snap.CurrentTPS)
	}
}

func TestRollingStatsErrorRateCarried(t *testing.T) {
	rs := NewRollingStats("run", "sub")
	rs.ObserveWindow(Aggregate{TPS: 100, Transactions: 90, Errors: 10, ErrorRate: 0.1})
	snap := rs.Snapshot()
	if snap.TotalErrors != 10 {
		t.Errorf("total errors = %d, want 10", snap.TotalErrors)
	}
	if math.Abs(snap.ErrorRate-0.1) > 1e-9 {
		t.Errorf("error rate = %.4f, want 0.1", snap.ErrorRate)
	}
}
