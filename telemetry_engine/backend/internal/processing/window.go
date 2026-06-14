// Package processing implements Track 3's stream-processing tier: it turns the
// raw, unbounded telemetry stream into bounded, time-windowed aggregates the
// scoring engine and dashboard consume.
//
// THREE WINDOW KINDS (Deliverable 5), all built on ONE slot ring:
//
//   - TUMBLING: fixed, non-overlapping intervals ("every 5s, here is that 5s").
//     Used for the authoritative, replayable per-interval record in TimescaleDB.
//
//   - SLIDING: an interval of width W recomputed every `slide` ("the last 5s,
//     refreshed each second"). Used to drive smooth live charts without waiting
//     a full window for an update.
//
//   - ROLLING: continuously-updated stats over the whole retention horizon
//     ("everything in the last 60s"). Used for stability (TPS variance) and the
//     current leaderboard number.
//
// All three are derived by summing a ring of fixed-granularity SLOTS. A slot is
// the smallest unit of time we bucket into (e.g. 1s); windows are just spans of
// consecutive slots. This keeps record O(1) and every window query O(#slots).
package processing

import (
	"time"

	"github.com/iicpc/track3/telemetry-engine/internal/percentile"
)

// HistLayout pins the HDR histogram parameters so every slot/window shares an
// identical bucket layout (required for exact Merge). Matches Track 2.
type HistLayout struct {
	Min     int64
	Max     int64
	Sigfigs int
}

// DefaultLayout is Track 2's layout: 1µs–10s, 3 significant digits.
var DefaultLayout = HistLayout{Min: 1, Max: 10_000_000, Sigfigs: 3}

func (l HistLayout) New() *percentile.Histogram { return percentile.New(l.Min, l.Max, l.Sigfigs) }

// slot is one granularity-sized time bucket. `epoch` is floor(t/granularity);
// a slot is "live" only while its epoch is within the ring's current horizon.
type slot struct {
	epoch    int64
	hist     *percentile.Histogram
	txns     uint64
	errors   uint64
	timeouts uint64
}

func (s *slot) reset(epoch int64) {
	s.epoch = epoch
	if s.hist == nil {
		return
	}
	s.hist.Reset()
	s.txns, s.errors, s.timeouts = 0, 0, 0
}

// Aggregate is the merged result of a span of slots — the value every window
// query returns. Latencies are microseconds.
type Aggregate struct {
	Start        time.Time
	End          time.Time
	Transactions uint64
	Errors       uint64
	Timeouts     uint64
	Seconds      float64
	TPS          float64
	ErrorRate    float64
	P50US        int64
	P90US        int64
	P99US        int64
	MaxUS        int64
	MeanUS       float64
	SampleCount  int64
}

// RollingAggregator maintains a ring of slots for ONE (run, submission). It is
// single-goroutine (the stream-processor shards by submission so each
// aggregator is touched by one worker) — no locks on the hot path, exactly like
// Track 2's per-shard collector.
type RollingAggregator struct {
	granularity time.Duration
	ringLen     int
	slots       []slot
	layout      HistLayout
	maxEpoch    int64 // highest epoch seen so far (the "now" of event time)
	lastTumble  int64 // last emitted tumbling block index
	tumbleSlots int64 // tumbling window width in slots
}

// NewRollingAggregator builds an aggregator whose ring covers `retention` at
// `granularity` resolution, and whose tumbling window is `tumbling` wide.
func NewRollingAggregator(granularity, retention, tumbling time.Duration, layout HistLayout) *RollingAggregator {
	if granularity <= 0 {
		granularity = time.Second
	}
	ringLen := int(retention / granularity)
	if ringLen < 1 {
		ringLen = 1
	}
	tumbleSlots := int64(tumbling / granularity)
	if tumbleSlots < 1 {
		tumbleSlots = 1
	}
	ra := &RollingAggregator{
		granularity: granularity,
		ringLen:     ringLen,
		slots:       make([]slot, ringLen),
		layout:      layout,
		lastTumble:  -1,
		tumbleSlots: tumbleSlots,
	}
	for i := range ra.slots {
		ra.slots[i].hist = layout.New()
		ra.slots[i].epoch = -1
	}
	return ra
}

func (ra *RollingAggregator) epochOf(t time.Time) int64 {
	return t.UnixNano() / int64(ra.granularity)
}

// slotFor returns the slot backing `epoch`, resetting it first if the ring
// position is currently occupied by a stale (expired) epoch.
func (ra *RollingAggregator) slotFor(epoch int64) *slot {
	pos := int(((epoch % int64(ra.ringLen)) + int64(ra.ringLen)) % int64(ra.ringLen))
	s := &ra.slots[pos]
	if s.epoch != epoch {
		s.reset(epoch)
	}
	return s
}

// Record ingests one order outcome at event-time `at`. latencyUS<0 means "no
// latency sample" (e.g. a pure error with no round trip).
func (ra *RollingAggregator) Record(at time.Time, latencyUS int64, isError, isTimeout bool) {
	epoch := ra.epochOf(at)
	if epoch > ra.maxEpoch {
		ra.maxEpoch = epoch
	}
	// Drop samples older than the retention horizon — they cannot affect any
	// live window and would alias onto a live ring slot.
	if epoch <= ra.maxEpoch-int64(ra.ringLen) {
		return
	}
	s := ra.slotFor(epoch)
	if latencyUS >= 0 {
		s.hist.Record(latencyUS)
	}
	if isError {
		s.errors++
	} else {
		s.txns++
	}
	if isTimeout {
		s.timeouts++
	}
}

// RecordPreaggregated merges a whole pre-computed window (Track 2's BotMetrics
// fast path) into the slot at `at`. The histogram counts are merged exactly so
// percentiles stay correct.
func (ra *RollingAggregator) RecordPreaggregated(at time.Time, hist *percentile.Histogram, txns, errors, timeouts uint64) {
	epoch := ra.epochOf(at)
	if epoch > ra.maxEpoch {
		ra.maxEpoch = epoch
	}
	if epoch <= ra.maxEpoch-int64(ra.ringLen) {
		return
	}
	s := ra.slotFor(epoch)
	if hist != nil {
		s.hist.Merge(hist)
	}
	s.txns += txns
	s.errors += errors
	s.timeouts += timeouts
}

// span merges slots whose epoch is in [lo, hi] (inclusive) into one Aggregate.
func (ra *RollingAggregator) span(lo, hi int64) Aggregate {
	merged := ra.layout.New()
	var txns, errors, timeouts uint64
	liveSlots := int64(0)
	for e := lo; e <= hi; e++ {
		if e < 0 {
			continue
		}
		pos := int(((e % int64(ra.ringLen)) + int64(ra.ringLen)) % int64(ra.ringLen))
		s := &ra.slots[pos]
		if s.epoch != e {
			continue // expired or never populated
		}
		merged.Merge(s.hist)
		txns += s.txns
		errors += s.errors
		timeouts += s.timeouts
		liveSlots++
	}
	seconds := float64(hi-lo+1) * ra.granularity.Seconds()
	if seconds <= 0 {
		seconds = ra.granularity.Seconds()
	}
	total := txns + errors
	var errRate float64
	if total > 0 {
		errRate = float64(errors) / float64(total)
	}
	return Aggregate{
		Start:        time.Unix(0, lo*int64(ra.granularity)),
		End:          time.Unix(0, (hi+1)*int64(ra.granularity)),
		Transactions: txns,
		Errors:       errors,
		Timeouts:     timeouts,
		Seconds:      seconds,
		TPS:          float64(txns) / seconds,
		ErrorRate:    errRate,
		P50US:        merged.ValueAtPercentile(50),
		P90US:        merged.ValueAtPercentile(90),
		P99US:        merged.ValueAtPercentile(99),
		MaxUS:        merged.Max(),
		MeanUS:       merged.Mean(),
		SampleCount:  merged.TotalCount(),
	}
}

// Sliding returns the window of `width` ending at the current event-time head.
// Called every flush tick to update live charts.
func (ra *RollingAggregator) Sliding(width time.Duration) Aggregate {
	w := int64(width / ra.granularity)
	if w < 1 {
		w = 1
	}
	hi := ra.maxEpoch
	lo := hi - w + 1
	return ra.span(lo, hi)
}

// Rolling returns the aggregate over the entire retention horizon. Basis for
// stability and the current leaderboard figure.
func (ra *RollingAggregator) Rolling() Aggregate {
	hi := ra.maxEpoch
	lo := hi - int64(ra.ringLen) + 1
	return ra.span(lo, hi)
}

// ClosedTumbling returns the most recently COMPLETED tumbling window, or
// (Aggregate{}, false) if none has closed since the last call. A tumbling block
// b spans epochs [b*tumbleSlots, (b+1)*tumbleSlots-1]; it is "closed" once the
// event-time head has advanced past its last epoch.
func (ra *RollingAggregator) ClosedTumbling() (Aggregate, bool) {
	currentBlock := ra.maxEpoch / ra.tumbleSlots
	// The last fully-closed block is currentBlock-1 (the current one is still
	// accumulating). Emit any blocks we haven't emitted yet, newest first.
	closed := currentBlock - 1
	if closed <= ra.lastTumble {
		return Aggregate{}, false
	}
	ra.lastTumble = closed
	lo := closed * ra.tumbleSlots
	hi := lo + ra.tumbleSlots - 1
	return ra.span(lo, hi), true
}

// MaxEpochTime is the event-time head, useful for logging/diagnostics.
func (ra *RollingAggregator) MaxEpochTime() time.Time {
	return time.Unix(0, ra.maxEpoch*int64(ra.granularity))
}
