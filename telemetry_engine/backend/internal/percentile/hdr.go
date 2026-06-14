// Package percentile implements an HDR (High Dynamic Range) histogram in Go,
// bucket-compatible with Track 2's C++ bot_fleet::metrics::HdrHistogram and
// faithful to Gil Tene's reference algorithm (same layout as the widely-used
// HdrHistogram/hdrhistogram-go port).
//
// WHY THIS EXISTS — the problem with averages (Deliverable 7):
//
//	A mean latency hides the tail. If 99 requests take 1ms and 1 takes 1000ms,
//	the mean is ~11ms — a number NO request actually experienced. Trading
//	engines live and die on the tail (p99): the slowest 1% of orders are
//	exactly the ones that lose money in a fast market. We therefore rank on
//	p50/p90/p99, never on the average.
//
// WHY A HISTOGRAM (not a sorted slice):
//
//	Exact percentiles need every sample sorted: O(n log n) time and O(n) memory
//	per window. At 100k+ orders/sec that is untenable. An HDR histogram records
//	in O(1) into fixed, logarithmically-spaced buckets, giving bounded memory
//	(~tens of KB) and percentiles accurate to a configurable number of
//	significant digits (here 3, i.e. ±0.1%).
//
// COORDINATED OMISSION (the subtle bug this guards against):
//
//	Naive load tests measure only requests that COMPLETED. When the system
//	stalls, in-flight requests pile up but aren't sampled, so the stall is
//	invisible — the histogram is "coordinated" with the system's bad behavior
//	and omits it. RecordWithExpectedInterval back-fills the samples a stall
//	implies, restoring an honest tail.
package percentile

import (
	"math"
	"math/bits"
)

// Histogram is a logarithmic-linear bucket histogram. It is NOT safe for
// concurrent use; give each goroutine/window its own and Merge them on a cold
// path (the same model Track 2 uses across shards).
type Histogram struct {
	lowestTrackableValue  int64
	highestTrackableValue int64
	unitMagnitude         int64
	significantFigures    int64

	subBucketHalfCountMagnitude int32
	subBucketHalfCount          int32
	subBucketMask               int64
	subBucketCount              int32
	bucketCount                 int32
	countsLen                   int32

	totalCount int64
	totalSum   int64 // for Mean()
	maxSeen    int64
	counts     []int64
}

// New builds a histogram covering [minValue, maxValue] (microseconds by
// convention) with sigfigs (1..5) of precision. Track-2-compatible defaults:
// New(1, 10_000_000, 3) — 1µs to 10s, 3 significant digits.
func New(minValue, maxValue int64, sigfigs int) *Histogram {
	if minValue < 1 {
		minValue = 1
	}
	if sigfigs < 1 {
		sigfigs = 1
	}
	if sigfigs > 5 {
		sigfigs = 5
	}

	largestValueWithSingleUnitResolution := 2 * math.Pow10(sigfigs)
	subBucketCountMagnitude := int32(math.Ceil(math.Log2(largestValueWithSingleUnitResolution)))

	subBucketHalfCountMagnitude := subBucketCountMagnitude
	if subBucketHalfCountMagnitude < 1 {
		subBucketHalfCountMagnitude = 1
	}
	subBucketHalfCountMagnitude--

	unitMagnitude := int32(math.Floor(math.Log2(float64(minValue))))
	if unitMagnitude < 0 {
		unitMagnitude = 0
	}

	subBucketCount := int32(math.Pow(2, float64(subBucketHalfCountMagnitude)+1))
	subBucketHalfCount := subBucketCount / 2
	subBucketMask := int64(subBucketCount-1) << uint(unitMagnitude)

	// Exponent range needed to track maxValue without overflow.
	smallestUntrackableValue := int64(subBucketCount) << uint(unitMagnitude)
	bucketsNeeded := int32(1)
	for smallestUntrackableValue < maxValue {
		smallestUntrackableValue <<= 1
		bucketsNeeded++
	}

	countsLen := (bucketsNeeded + 1) * (subBucketCount / 2)

	return &Histogram{
		lowestTrackableValue:        minValue,
		highestTrackableValue:       maxValue,
		unitMagnitude:               int64(unitMagnitude),
		significantFigures:          int64(sigfigs),
		subBucketHalfCountMagnitude: subBucketHalfCountMagnitude,
		subBucketHalfCount:          subBucketHalfCount,
		subBucketMask:               subBucketMask,
		subBucketCount:              subBucketCount,
		bucketCount:                 bucketsNeeded,
		countsLen:                   countsLen,
		counts:                      make([]int64, countsLen),
	}
}

// NewDefault returns the canonical Track 3 histogram: 1µs–10s, 3 sig digits.
func NewDefault() *Histogram { return New(1, 10_000_000, 3) }

func bitLen(x int64) int32 {
	return int32(64 - bits.LeadingZeros64(uint64(x)))
}

func (h *Histogram) getBucketIndex(v int64) int32 {
	pow2Ceiling := bitLen(v | h.subBucketMask) // index of highest set bit (+1)
	return pow2Ceiling - int32(h.unitMagnitude) - (h.subBucketHalfCountMagnitude + 1)
}

func (h *Histogram) getSubBucketIndex(v int64, bucketIndex int32) int32 {
	return int32(v >> uint(int64(bucketIndex)+h.unitMagnitude))
}

func (h *Histogram) countsIndex(bucketIndex, subBucketIndex int32) int32 {
	bucketBaseIndex := (bucketIndex + 1) << uint(h.subBucketHalfCountMagnitude)
	offsetInBucket := subBucketIndex - h.subBucketHalfCount
	return bucketBaseIndex + offsetInBucket
}

func (h *Histogram) countsIndexFor(v int64) int32 {
	bucketIndex := h.getBucketIndex(v)
	subBucketIndex := h.getSubBucketIndex(v, bucketIndex)
	return h.countsIndex(bucketIndex, subBucketIndex)
}

func (h *Histogram) valueFromIndex(bucketIndex, subBucketIndex int32) int64 {
	return int64(subBucketIndex) << uint(int64(bucketIndex)+h.unitMagnitude)
}

func (h *Histogram) valueFromCountsIndex(index int32) int64 {
	bucketIndex := (index >> h.subBucketHalfCountMagnitude) - 1
	subBucketIndex := (index & (h.subBucketHalfCount - 1)) + h.subBucketHalfCount
	if bucketIndex < 0 {
		subBucketIndex -= h.subBucketHalfCount
		bucketIndex = 0
	}
	return h.valueFromIndex(bucketIndex, subBucketIndex)
}

// sizeOfEquivalentValueRange returns the width of the bucket containing v.
func (h *Histogram) sizeOfEquivalentValueRange(v int64) int64 {
	bucketIndex := h.getBucketIndex(v)
	subBucketIndex := h.getSubBucketIndex(v, bucketIndex)
	adjustedBucket := bucketIndex
	if subBucketIndex >= h.subBucketCount {
		adjustedBucket++
	}
	return int64(1) << uint(h.unitMagnitude+int64(adjustedBucket))
}

func (h *Histogram) lowestEquivalentValue(v int64) int64 {
	bucketIndex := h.getBucketIndex(v)
	subBucketIndex := h.getSubBucketIndex(v, bucketIndex)
	return h.valueFromIndex(bucketIndex, subBucketIndex)
}

func (h *Histogram) nextNonEquivalentValue(v int64) int64 {
	return h.lowestEquivalentValue(v) + h.sizeOfEquivalentValueRange(v)
}

func (h *Histogram) highestEquivalentValue(v int64) int64 {
	return h.nextNonEquivalentValue(v) - 1
}

// Record adds one sample. O(1). Values above the trackable max are clamped to
// max so an outlier lands in the top bucket rather than being dropped.
func (h *Histogram) Record(value int64) { h.RecordN(value, 1) }

// RecordN adds `count` identical samples at once (used when merging or when a
// pre-aggregated bucket arrives over the bus).
func (h *Histogram) RecordN(value, count int64) {
	if value < 0 || count <= 0 {
		return
	}
	if value > h.highestTrackableValue {
		value = h.highestTrackableValue
	}
	idx := h.countsIndexFor(value)
	if idx < 0 || idx >= h.countsLen {
		return
	}
	h.counts[idx] += count
	h.totalCount += count
	h.totalSum += value * count
	if value > h.maxSeen {
		h.maxSeen = value
	}
}

// RecordWithExpectedInterval records `value`, then — if value exceeds the
// expected interval between samples — synthesizes the extra samples a fair
// (uncoordinated) load generator WOULD have recorded during the stall. This is
// the standard correction for COORDINATED OMISSION.
//
// Example: expectedInterval=1ms, observed=5ms. A fair generator would have
// issued requests at t=0,1,2,3,4ms; those queued behind the slow one "should"
// have measured 4,3,2,1ms. We back-fill them so the tail reflects the real
// stall instead of hiding it.
func (h *Histogram) RecordWithExpectedInterval(value, expectedInterval int64) {
	h.Record(value)
	if expectedInterval <= 0 || value <= expectedInterval {
		return
	}
	for missing := value - expectedInterval; missing >= expectedInterval; missing -= expectedInterval {
		h.Record(missing)
	}
}

// ValueAtPercentile returns the value at percentile p (0..100). Primary read
// path for the scoring engine and dashboard.
func (h *Histogram) ValueAtPercentile(p float64) int64 {
	if h.totalCount == 0 {
		return 0
	}
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	countAtPercentile := int64(((p / 100.0) * float64(h.totalCount)) + 0.5)
	if countAtPercentile < 1 {
		countAtPercentile = 1
	}
	var total int64
	for i := int32(0); i < h.countsLen; i++ {
		total += h.counts[i]
		if total >= countAtPercentile {
			return h.highestEquivalentValue(h.valueFromCountsIndex(i))
		}
	}
	return 0
}

// Mean returns the arithmetic mean. Informational only — never used for ranking
// (see the package doc on why averages mislead).
func (h *Histogram) Mean() float64 {
	if h.totalCount == 0 {
		return 0
	}
	return float64(h.totalSum) / float64(h.totalCount)
}

func (h *Histogram) TotalCount() int64 { return h.totalCount }
func (h *Histogram) Max() int64        { return h.maxSeen }

// Reset clears all counts for reuse at a window boundary.
func (h *Histogram) Reset() {
	for i := range h.counts {
		h.counts[i] = 0
	}
	h.totalCount = 0
	h.totalSum = 0
	h.maxSeen = 0
}

// Merge adds another histogram's counts into this one. Both MUST share the same
// layout (same New(...) parameters). Merging is exact and additive: the merged
// percentiles equal what one histogram fed every sample would report — which is
// why Track 2 can shard recording and Track 3 can re-merge shard histograms.
func (h *Histogram) Merge(other *Histogram) {
	if other == nil {
		return
	}
	n := h.countsLen
	if other.countsLen < n {
		n = other.countsLen
	}
	for i := int32(0); i < n; i++ {
		if c := other.counts[i]; c != 0 {
			h.counts[i] += c
			h.totalCount += c
			h.totalSum += h.valueFromCountsIndex(i) * c
		}
	}
	if other.maxSeen > h.maxSeen {
		h.maxSeen = other.maxSeen
	}
}

// Counts returns a copy of the raw bucket array, for serialization across the
// bus so the stream processor can re-merge shard histograms exactly.
func (h *Histogram) Counts() []int64 {
	out := make([]int64, len(h.counts))
	copy(out, h.counts)
	return out
}

// LoadCounts restores a histogram from a Counts() array produced by a histogram
// with identical layout.
func (h *Histogram) LoadCounts(counts []int64) {
	h.Reset()
	n := int32(len(counts))
	if n > h.countsLen {
		n = h.countsLen
	}
	for i := int32(0); i < n; i++ {
		if counts[i] != 0 {
			h.counts[i] = counts[i]
			h.totalCount += counts[i]
			v := h.valueFromCountsIndex(i)
			h.totalSum += v * counts[i]
			if v > h.maxSeen {
				h.maxSeen = v
			}
		}
	}
}
