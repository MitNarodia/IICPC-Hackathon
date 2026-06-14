package scoring

import "testing"

func TestLatencyScoreMonotonic(t *testing.T) {
	c := DefaultConfig()
	fast := LatencyScore(100, c.BestP99US, c.WorstP99US)   // at best ref
	mid := LatencyScore(2000, c.BestP99US, c.WorstP99US)    // 2ms
	slow := LatencyScore(50000, c.BestP99US, c.WorstP99US)  // at worst ref
	if !(fast > mid && mid > slow) {
		t.Fatalf("latency score should decrease with latency: fast=%.1f mid=%.1f slow=%.1f", fast, mid, slow)
	}
	if fast < 99.9 {
		t.Errorf("best latency should score ~100, got %.2f", fast)
	}
	if slow > 0.1 {
		t.Errorf("worst latency should score ~0, got %.2f", slow)
	}
}

func TestThroughputSaturates(t *testing.T) {
	if got := ThroughputScore(200_000, 100_000); got != 100 {
		t.Errorf("throughput beyond target should clamp to 100, got %.2f", got)
	}
	if got := ThroughputScore(50_000, 100_000); got != 50 {
		t.Errorf("half target should score 50, got %.2f", got)
	}
}

func TestStabilityPenalizesVarianceAndErrors(t *testing.T) {
	steady := StabilityScore(0.0, 0.0)
	bursty := StabilityScore(0.5, 0.0)
	errs := StabilityScore(0.0, 0.3)
	if steady != 100 {
		t.Errorf("perfectly steady, error-free should score 100, got %.2f", steady)
	}
	if !(bursty < steady) || !(errs < steady) {
		t.Errorf("variance and errors must lower stability: bursty=%.1f errs=%.1f", bursty, errs)
	}
}

// A fast but incorrect engine must NOT beat a slightly slower, correct one.
func TestCorrectnessGateBeatsRawSpeed(t *testing.T) {
	c := DefaultConfig()
	fastWrong := c.Compute(Inputs{TPS: 100_000, P99US: 100, ErrorRate: 0, Correctness: 0.0, TPSCoV: 0})
	solidRight := c.Compute(Inputs{TPS: 60_000, P99US: 800, ErrorRate: 0.0, Correctness: 1.0, TPSCoV: 0.05})
	if fastWrong.Composite >= solidRight.Composite {
		t.Fatalf("incorrect engine should not outrank correct one: wrong=%.2f right=%.2f",
			fastWrong.Composite, solidRight.Composite)
	}
}

func TestCompositeBounds(t *testing.T) {
	c := DefaultConfig()
	best := c.Compute(Inputs{TPS: 1_000_000, P99US: 50, ErrorRate: 0, Correctness: 1, TPSCoV: 0})
	worst := c.Compute(Inputs{TPS: 0, P99US: 1_000_000, ErrorRate: 1, Correctness: 0, TPSCoV: 5})
	if best.Composite > 100.01 || worst.Composite < -0.01 {
		t.Fatalf("composite out of [0,100]: best=%.2f worst=%.2f", best.Composite, worst.Composite)
	}
}
