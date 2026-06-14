#pragma once

/// metrics/metrics_collector.hpp
/// =============================
/// WHY THIS FILE EXISTS:
///   Per-shard telemetry. After the thread-per-core redesign, every worker
///   thread owns ONE MetricsCollector. Because a shard is single-threaded,
///   the hot path (record_*) touches only thread-private memory: no atomics,
///   no locks, no cross-core cache-line bouncing.
///
///   Cross-shard aggregation is handled separately by MetricsAggregator.
///   A shard periodically produces a MetricsSnapshot (on its OWN thread, so
///   the copy is race-free) and hands it to the aggregator on a cold path.
///
/// CLASSES:
///   MetricsSnapshot  — A race-free, movable copy of one window's telemetry.
///   MetricsCollector — Per-shard recorder; produces snapshots.
///
/// THREADING MODEL:
///   - record_latency()/record_transaction()/... run on the shard's io_context
///     thread only. Single writer => plain (non-atomic) counters are correct.
///   - snapshot_and_reset() also runs on the shard's own thread, so reading
///     and resetting the histogram + counters cannot race with record_*.
///
/// QUEUES: None. Direct histogram insertion.
///
/// ASYNC OPS: None. Driven by the shard's metrics timer coroutine.

#include "metrics/hdr_histogram.hpp"
#include <chrono>
#include <cstdint>

namespace bot_fleet::metrics {

/// One window's worth of telemetry, detached from any collector so it can be
/// safely moved across threads to the aggregator.
struct MetricsSnapshot {
    HdrHistogram hist{1, 10'000'000, 3};  // same bucket layout as collectors
    uint64_t txns = 0;
    uint64_t errors = 0;
    uint64_t timeouts = 0;
    double   seconds = 0.0;                // wall time covered by this window
};

class MetricsCollector {
public:
    MetricsCollector();

    /// Record a single order round-trip latency (in microseconds).
    /// Hot path, shard-thread only. O(1), no locks, no atomics.
    void record_latency(int64_t latency_us);

    /// Throughput counters. Hot path, shard-thread only.
    void record_transaction();
    void record_error();
    void record_timeout();

    /// Detach the current window as a snapshot and reset for the next window.
    /// MUST be called on the shard's own thread (race-free w.r.t. record_*).
    MetricsSnapshot snapshot_and_reset();

private:
    HdrHistogram histogram_;

    // Single-writer-per-shard => plain integers, no atomics required.
    uint64_t window_transactions_ = 0;
    uint64_t window_errors_ = 0;
    uint64_t window_timeouts_ = 0;

    // Timing for TPS calculation
    std::chrono::steady_clock::time_point window_start_;
};

} // namespace bot_fleet::metrics
