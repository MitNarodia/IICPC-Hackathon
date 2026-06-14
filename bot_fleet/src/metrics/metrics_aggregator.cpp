/// metrics/metrics_aggregator.cpp
/// ==============================
/// Implements the thread-safe cross-shard merge and reporting.
///
/// CORRECTNESS NOTE:
///   HdrHistogram::merge is additive on a fixed bucket layout, so merging N
///   shard histograms yields exactly the same bucket counts (and therefore
///   the same percentiles) as if every sample had been recorded into one
///   histogram. Sharding does not bias the aggregate distribution.

#include "metrics/metrics_aggregator.hpp"

#include <algorithm>
#include <iomanip>
#include <iostream>

namespace bot_fleet::metrics {

MetricsAggregator::MetricsAggregator(unsigned num_shards)
    : num_shards_(num_shards)
{}

void MetricsAggregator::submit(MetricsSnapshot snap) {
    std::lock_guard<std::mutex> lk(mtx_);

    // Rolling window view.
    window_hist_.merge(snap.hist);
    window_txns_ += snap.txns;
    window_errors_ += snap.errors;
    window_timeouts_ += snap.timeouts;
    // Shards run concurrently, so the window's wall time is the longest
    // shard window, not the sum.
    window_seconds_ = std::max(window_seconds_, snap.seconds);

    // Cumulative view.
    cumulative_hist_.merge(snap.hist);
    total_txns_ += snap.txns;
    total_errors_ += snap.errors;
    total_timeouts_ += snap.timeouts;
    total_seconds_ = std::max(total_seconds_, snap.seconds);
}

void MetricsAggregator::flush() {
    std::lock_guard<std::mutex> lk(mtx_);

    if (window_hist_.total_count() == 0 && window_txns_ == 0) {
        return;  // nothing new this interval
    }

    print_locked("LIVE WINDOW (all shards merged)",
                 window_hist_, window_txns_, window_errors_, window_timeouts_,
                 window_seconds_);

    // Reset rolling window for the next interval.
    window_hist_.reset();
    window_txns_ = 0;
    window_errors_ = 0;
    window_timeouts_ = 0;
    window_seconds_ = 0.0;
}

void MetricsAggregator::final_report() {
    std::lock_guard<std::mutex> lk(mtx_);
    print_locked("FINAL CUMULATIVE (whole run)",
                 cumulative_hist_, total_txns_, total_errors_, total_timeouts_,
                 total_seconds_);
}

AggregateView MetricsAggregator::cumulative() const {
    std::lock_guard<std::mutex> lk(mtx_);
    AggregateView v;
    v.txns     = total_txns_;
    v.errors   = total_errors_;
    v.timeouts = total_timeouts_;
    v.seconds  = total_seconds_;
    v.p50      = cumulative_hist_.percentile(50.0);
    v.p90      = cumulative_hist_.percentile(90.0);
    v.p99      = cumulative_hist_.percentile(99.0);
    v.mean     = cumulative_hist_.mean();
    return v;
}

AggregateView MetricsAggregator::window() const {
    std::lock_guard<std::mutex> lk(mtx_);
    AggregateView v;
    v.txns     = window_txns_;
    v.errors   = window_errors_;
    v.timeouts = window_timeouts_;
    v.seconds  = window_seconds_;
    v.p50      = window_hist_.percentile(50.0);
    v.p90      = window_hist_.percentile(90.0);
    v.p99      = window_hist_.percentile(99.0);
    v.mean     = window_hist_.mean();
    return v;
}

void MetricsAggregator::print_locked(const char* title,
                                     const HdrHistogram& hist,
                                     uint64_t txns, uint64_t errors,
                                     uint64_t timeouts, double seconds) {
    const double tps = (seconds > 0.0)
        ? static_cast<double>(txns) / seconds
        : 0.0;

    std::cout << "\n==================== " << title << " ====================\n";
    std::cout << "  Shards:        " << num_shards_ << "\n";
    std::cout << "  Window (s):    " << std::fixed << std::setprecision(2) << seconds << "\n";
    std::cout << "  Transactions:  " << txns << "\n";
    std::cout << "  Errors:        " << errors << "\n";
    std::cout << "  Timeouts:      " << timeouts << "\n";
    std::cout << "  Aggregate TPS: " << std::fixed << std::setprecision(1) << tps << "\n";
    std::cout << "  Latency (us):  "
              << "p50=" << hist.percentile(50.0) << "  "
              << "p90=" << hist.percentile(90.0) << "  "
              << "p99=" << hist.percentile(99.0) << "  "
              << "mean=" << static_cast<int64_t>(hist.mean()) << "\n";
    std::cout << "==========================================================\n";
}

} // namespace bot_fleet::metrics
