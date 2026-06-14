package percentile

import (
	"math"
	"testing"
)

// TestLinearValues feeds 1..10000µs once each and checks the percentiles land
// within the histogram's significant-figure tolerance. With a uniform
// distribution, the value at percentile p should be ~p% of the max.
func TestLinearValues(t *testing.T) {
	h := NewDefault()
	const n = 10000
	for v := int64(1); v <= n; v++ {
		h.Record(v)
	}
	if h.TotalCount() != n {
		t.Fatalf("total count = %d, want %d", h.TotalCount(), n)
	}
	cases := []struct {
		p    float64
		want int64
	}{
		{50, 5000},
		{90, 9000},
		{99, 9900},
		{100, 10000},
	}
	for _, c := range cases {
		got := h.ValueAtPercentile(c.p)
		// 3 sig figs => ±0.1% of value tolerance, plus 1 bucket of slack.
		tol := int64(float64(c.want)*0.01) + 1
		if got < c.want-tol || got > c.want+tol {
			t.Errorf("p%.0f = %d, want ~%d (±%d)", c.p, got, c.want, tol)
		}
	}
}

// TestTailIsNotHiddenByMean is the headline correctness property: an outlier
// dominates the tail percentile but barely moves the mean. This is the whole
// reason we rank on p99, not average.
func TestTailIsNotHiddenByMean(t *testing.T) {
	h := NewDefault()
	for i := 0; i < 99; i++ {
		h.Record(1000) // 99 fast samples at 1ms
	}
	h.Record(1_000_000) // 1 slow sample at 1s

	p99 := h.ValueAtPercentile(99)
	mean := h.Mean()

	if p99 < 900_000 {
		t.Errorf("p99 = %d, expected the slow sample to dominate the tail", p99)
	}
	if mean > 50_000 {
		t.Errorf("mean = %.0f, expected the average to stay small and misleading", mean)
	}
}

// TestMergeEqualsSingle proves the sharding-then-merge property Track 2 relies
// on: recording across two histograms and merging yields identical percentiles
// to recording everything into one.
func TestMergeEqualsSingle(t *testing.T) {
	single := NewDefault()
	a := NewDefault()
	b := NewDefault()
	for v := int64(1); v <= 20000; v++ {
		single.Record(v)
		if v%2 == 0 {
			a.Record(v)
		} else {
			b.Record(v)
		}
	}
	a.Merge(b)
	for _, p := range []float64{50, 90, 99, 99.9} {
		if a.ValueAtPercentile(p) != single.ValueAtPercentile(p) {
			t.Errorf("p%.1f merged=%d single=%d", p, a.ValueAtPercentile(p), single.ValueAtPercentile(p))
		}
	}
	if a.TotalCount() != single.TotalCount() {
		t.Errorf("merged count=%d single=%d", a.TotalCount(), single.TotalCount())
	}
}

// TestCountsRoundTrip verifies the serialize/deserialize path used to ship
// histograms across Redpanda for exact cross-shard merging.
func TestCountsRoundTrip(t *testing.T) {
	src := NewDefault()
	for v := int64(10); v <= 5000; v += 7 {
		src.Record(v)
	}
	dst := NewDefault()
	dst.LoadCounts(src.Counts())

	if dst.TotalCount() != src.TotalCount() {
		t.Fatalf("count mismatch: %d vs %d", dst.TotalCount(), src.TotalCount())
	}
	for _, p := range []float64{50, 90, 99} {
		if dst.ValueAtPercentile(p) != src.ValueAtPercentile(p) {
			t.Errorf("p%.0f mismatch after round trip: %d vs %d", p, dst.ValueAtPercentile(p), src.ValueAtPercentile(p))
		}
	}
}

// TestCoordinatedOmission shows the correction inflates the tail when a stall
// is present, versus the naive recording that hides it.
func TestCoordinatedOmission(t *testing.T) {
	const expected = 1000 // expected 1ms cadence

	naive := NewDefault()
	corrected := NewDefault()

	// 1000 well-behaved samples at 1ms.
	for i := 0; i < 1000; i++ {
		naive.Record(expected)
		corrected.RecordWithExpectedInterval(expected, expected)
	}
	// One catastrophic 100ms stall.
	naive.Record(100 * expected)
	corrected.RecordWithExpectedInterval(100*expected, expected)

	if corrected.ValueAtPercentile(99) <= naive.ValueAtPercentile(99) {
		t.Errorf("coordinated-omission correction should raise p99: corrected=%d naive=%d",
			corrected.ValueAtPercentile(99), naive.ValueAtPercentile(99))
	}
}

// TestEmpty guards the zero-sample edge case (no panics, sane zeros).
func TestEmpty(t *testing.T) {
	h := NewDefault()
	if h.ValueAtPercentile(50) != 0 || h.Mean() != 0 || h.TotalCount() != 0 {
		t.Errorf("empty histogram should report zeros")
	}
}

func approxEqual(a, b, tol float64) bool { return math.Abs(a-b) <= tol }
