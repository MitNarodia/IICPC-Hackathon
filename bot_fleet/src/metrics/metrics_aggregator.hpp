#pragma once

/// metrics/metrics_aggregator.hpp
/// ==============================
/// WHY THIS FILE EXISTS:
///   After the thread-per-core redesign there is no single hot-path metrics
///   object. Each shard records into its own MetricsCollector. This class
///   merges those per-shard windows into a single global view for the live
///   leaderboard, and into a cumulative view for the authoritative end-of-run
///   report.
///
/// CLASSES:
///   MetricsAggregator — Thread-safe merge point for per-shard snapshots.
///
/// THREADING MODEL:
///   - submit() is called by each shard's metrics coroutine, on the shard's
///     OWN thread, once per reporting window (a COLD path: ~once/5s/shard).
///   - flush() is called by a dedicated reporter thread on its own timer.
///   - final_report() is called once by the main thread AFTER all worker
///     threads have joined (so no submit() can be in flight).
///   All three take a single mutex. Because they are all cold paths, this
///   mutex never serializes the bot hot path — that was the whole point of
///   moving to per-shard collectors.
///
/// QUEUES: None. Snapshots are merged on arrival.
///
/// ASYNC OPS: None. Plain synchronous, mutex-guarded merges.

#include "metrics/metrics_collector.hpp"
#include "metrics/hdr_histogram.hpp"

#include <cstdint>
#include <mutex>

namespace bot_fleet::metrics {

/// Read-only, programmatic view of a merged metrics window. Lets the test
/// suite (and a future live leaderboard / Track-3 ingestion layer) consume
/// aggregated results directly instead of scraping the human-readable stdout.
struct AggregateView {
    uint64_t txns = 0;
    uint64_t errors = 0;
    uint64_t timeouts = 0;
    double   seconds = 0.0;
    int64_t  p50 = 0;
    int64_t  p90 = 0;
    int64_t  p99 = 0;
    double   mean = 0.0;
};

class MetricsAggregator {
public:
    explicit MetricsAggregator(unsigned num_shards);

    /// Merge one shard's window snapshot. Thread-safe (cold path).
    void submit(MetricsSnapshot snap);

    /// Print and reset the rolling window view. Called by the reporter thread.
    void flush();

    /// Print the cumulative whole-run view. Called once after all joins.
    void final_report();

    /// Snapshot the cumulative (whole-run) merged view. Thread-safe.
    AggregateView cumulative() const;

    /// Snapshot the current rolling-window merged view. Thread-safe.
    AggregateView window() const;

private:
    void print_locked(const char* title,
                      const HdrHistogram& hist,
                      uint64_t txns, uint64_t errors, uint64_t timeouts,
                      double seconds);

    mutable std::mutex mtx_;

    // Rolling window: reset every flush().
    HdrHistogram window_hist_{1, 10'000'000, 3};
    uint64_t window_txns_ = 0;
    uint64_t window_errors_ = 0;
    uint64_t window_timeouts_ = 0;
    double   window_seconds_ = 0.0;

    // Cumulative: never reset; basis for the final authoritative percentiles.
    HdrHistogram cumulative_hist_{1, 10'000'000, 3};
    uint64_t total_txns_ = 0;
    uint64_t total_errors_ = 0;
    uint64_t total_timeouts_ = 0;
    double   total_seconds_ = 0.0;

    unsigned num_shards_;
};

} // namespace bot_fleet::metrics
