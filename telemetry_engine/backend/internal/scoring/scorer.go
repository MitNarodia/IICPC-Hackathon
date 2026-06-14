// Package scoring turns raw window/validation signals into a single composite
// score in [0,100] that ranks the leaderboard.
//
// DESIGN — absolute, reference-based scoring (Deliverable 8):
//
//	We score each submission against FIXED reference points, NOT against the
//	current field. Two reasons:
//	  1. Streaming-friendly: we never need to know every competitor to score one
//	     — important when runs start/stop independently and partitions are
//	     processed in isolation.
//	  2. Stable & explainable: a submission's score reflects ITS OWN behaviour,
//	     so it cannot drop just because a faster entrant appeared. Judges can
//	     read the curve and reproduce the number by hand.
//
//	Tradeoff vs RELATIVE (rank/percentile) scoring: relative scoring spreads the
//	field out maximally and is "fairer" when references are mis-set, but it is
//	non-local (needs all data), unstable (scores move when others change), and
//	harder to explain. We deliberately chose absolute and expose the reference
//	points as config so they can be tuned per contest.
//
// THE FOUR AXES:
//
//	latency      — lower p99 is better, scored on a LOG curve (latency spans
//	               orders of magnitude; linear would ignore the tail we care about)
//	throughput   — higher successful TPS is better, linear up to a target
//	correctness  — the validation engine's correctness ratio, passed straight through
//	stability    — steadiness: penalizes TPS variance (CoV) AND error rate
//
//	A fast but WRONG engine is worthless, so correctness is both a heavily-
//	weighted axis AND an optional multiplicative gate (CorrectnessGate).
package scoring

import "math"

// Config holds weights and reference points. Weights need not sum to 1 — the
// composite normalizes by their sum.
type Config struct {
	WeightLatency     float64
	WeightThroughput  float64
	WeightCorrectness float64
	WeightStability   float64

	// Latency reference (microseconds). p99 at or below BestP99US scores 100 on
	// the latency axis; at or above WorstP99US scores 0.
	BestP99US  float64
	WorstP99US float64

	// Throughput reference: TPS at or above TargetTPS scores 100.
	TargetTPS float64

	// CorrectnessGate, when in (0,1], multiplies the composite by
	// correctness^gate-exponent-equivalent — see Compute. 0 disables the gate.
	CorrectnessGate float64
}

// DefaultConfig is tuned for a microsecond-latency matching-engine contest.
func DefaultConfig() Config {
	return Config{
		WeightLatency:     0.35,
		WeightThroughput:  0.30,
		WeightCorrectness: 0.25,
		WeightStability:   0.10,
		BestP99US:         100,      // 100µs p99 → full latency marks
		WorstP99US:        50_000,   // 50ms p99 → zero latency marks
		TargetTPS:         100_000,  // 100k successful TPS → full throughput marks
		CorrectnessGate:   0.5,      // halve a fully-incorrect engine's composite
	}
}

// Inputs are the joined signals for one submission at scoring time.
type Inputs struct {
	TPS         float64
	P99US       int64
	ErrorRate   float64 // 0..1
	Correctness float64 // 0..1 from the validation engine
	TPSCoV      float64 // coefficient of variation of per-window TPS (0..∞)
}

// Components is the breakdown the dashboard shows, each in [0,100].
type Components struct {
	Latency     float64
	Throughput  float64
	Correctness float64
	Stability   float64
	Composite   float64
}

// clamp01 bounds x to [0,1].
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// LatencyScore maps p99 (µs) to [0,100] on a log scale between the best and
// worst references. Lower latency → higher score.
func LatencyScore(p99us float64, best, worst float64) float64 {
	if p99us <= 0 {
		return 0 // no data yet
	}
	if best <= 0 {
		best = 1
	}
	if worst <= best {
		worst = best * 100
	}
	x := p99us
	if x < best {
		x = best
	}
	if x > worst {
		x = worst
	}
	// 1.0 at best, 0.0 at worst, log-interpolated in between.
	s := (math.Log10(worst) - math.Log10(x)) / (math.Log10(worst) - math.Log10(best))
	return 100 * clamp01(s)
}

// ThroughputScore maps successful TPS to [0,100], linear up to the target.
func ThroughputScore(tps, target float64) float64 {
	if target <= 0 {
		return 0
	}
	return 100 * clamp01(tps/target)
}

// StabilityScore rewards steady throughput and few errors. CoV and error rate
// each subtract from a perfect 1.0; the worse of the two dominates.
func StabilityScore(cov, errorRate float64) float64 {
	return 100 * clamp01(1.0-clamp01(cov)-clamp01(errorRate))
}

// Compute produces the four sub-scores and the gated, weighted composite.
func (c Config) Compute(in Inputs) Components {
	lat := LatencyScore(float64(in.P99US), c.BestP99US, c.WorstP99US)
	thr := ThroughputScore(in.TPS, c.TargetTPS)
	cor := 100 * clamp01(in.Correctness)
	stb := StabilityScore(in.TPSCoV, in.ErrorRate)

	wsum := c.WeightLatency + c.WeightThroughput + c.WeightCorrectness + c.WeightStability
	if wsum <= 0 {
		wsum = 1
	}
	composite := (c.WeightLatency*lat + c.WeightThroughput*thr +
		c.WeightCorrectness*cor + c.WeightStability*stb) / wsum

	// Correctness gate: scale composite toward zero as correctness falls, so a
	// blazing-fast but incorrect engine cannot top the board. gate=0.5 means a
	// fully-incorrect engine keeps 50% of its otherwise-earned composite; a
	// fully-correct engine is unaffected.
	if c.CorrectnessGate > 0 {
		gate := (1 - c.CorrectnessGate) + c.CorrectnessGate*clamp01(in.Correctness)
		composite *= gate
	}

	return Components{
		Latency:     round2(lat),
		Throughput:  round2(thr),
		Correctness: round2(cor),
		Stability:   round2(stb),
		Composite:   round2(composite),
	}
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
