package processing

import (
	"math"
	"testing"
	"time"
)

// A tumbling block must close exactly once the event-time head passes its last
// slot, and report the right txn count, TPS, and percentile.
func TestTumblingAggregates(t *testing.T) {
	ra := NewRollingAggregator(time.Second, 20*time.Second, 5*time.Second, DefaultLayout)
	// Block 100 spans epochs 500..504 (granularity 1s, tumbling 5s).
	for i := 0; i < 10; i++ {
		ra.Record(time.Unix(500+int64(i%5), 0), 1000, false, false)
	}
	// Advance the head into block 101 so block 100 is fully closed.
	ra.Record(time.Unix(505, 0), 1000, false, false)

	agg, ok := ra.ClosedTumbling()
	if !ok {
		t.Fatal("expected a closed tumbling window")
	}
	if agg.Transactions != 10 {
		t.Fatalf("transactions = %d, want 10", agg.Transactions)
	}
	if agg.TPS < 1.99 || agg.TPS > 2.01 {
		t.Errorf("tps = %.3f, want ~2.0 (10 txns / 5s)", agg.TPS)
	}
	if agg.P50US < 950 || agg.P50US > 1050 {
		t.Errorf("p50 = %d µs, want ~1000", agg.P50US)
	}
	// A second call with no further advance yields nothing new.
	if _, ok := ra.ClosedTumbling(); ok {
		t.Error("expected no additional closed window")
	}
}

func TestErrorRateAndTimeouts(t *testing.T) {
	ra := NewRollingAggregator(time.Second, 20*time.Second, 5*time.Second, DefaultLayout)
	for i := 0; i < 8; i++ {
		ra.Record(time.Unix(600, 0), 1000, false, false)
	}
	for i := 0; i < 2; i++ {
		ra.Record(time.Unix(600, 0), -1, true, false)
	}
	ra.Record(time.Unix(600, 0), -1, true, true) // one more error, also a timeout

	roll := ra.Rolling()
	if roll.Transactions != 8 {
		t.Errorf("transactions = %d, want 8", roll.Transactions)
	}
	if roll.Errors != 3 {
		t.Errorf("errors = %d, want 3", roll.Errors)
	}
	if roll.Timeouts != 1 {
		t.Errorf("timeouts = %d, want 1", roll.Timeouts)
	}
	want := 3.0 / 11.0
	if math.Abs(roll.ErrorRate-want) > 0.001 {
		t.Errorf("error_rate = %.4f, want %.4f", roll.ErrorRate, want)
	}
}

// Samples older than the retention horizon must not survive in the ring.
func TestRetentionDropsOldSamples(t *testing.T) {
	ra := NewRollingAggregator(time.Second, 10*time.Second, 5*time.Second, DefaultLayout)
	ra.Record(time.Unix(1000, 0), 1000, false, false) // head → epoch 1000
	ra.Record(time.Unix(1020, 0), 1000, false, false) // head → epoch 1020 (old expired)

	roll := ra.Rolling()
	if roll.Transactions != 1 {
		t.Errorf("transactions = %d, want 1 (stale sample dropped)", roll.Transactions)
	}
}

// The sliding window only includes the most recent `width`.
func TestSlidingWindowBounded(t *testing.T) {
	ra := NewRollingAggregator(time.Second, 60*time.Second, 5*time.Second, DefaultLayout)
	// 30 events spread one-per-second across epochs 700..729.
	for i := 0; i < 30; i++ {
		ra.Record(time.Unix(700+int64(i), 0), 500, false, false)
	}
	sl := ra.Sliding(5 * time.Second)
	// Head is epoch 729; sliding 5s covers epochs 725..729 = 5 events.
	if sl.Transactions != 5 {
		t.Errorf("sliding transactions = %d, want 5", sl.Transactions)
	}
	if full := ra.Rolling(); full.Transactions != 30 {
		t.Errorf("rolling transactions = %d, want 30", full.Transactions)
	}
}
