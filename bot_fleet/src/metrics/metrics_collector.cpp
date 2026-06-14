/// metrics/metrics_collector.cpp
/// =============================
/// Per-shard recorder. Hot path is single-threaded (shard-local), so no
/// atomics or locks are used. Aggregation across shards is done elsewhere.

#include "metrics/metrics_collector.hpp"

namespace bot_fleet::metrics {

MetricsCollector::MetricsCollector()
    : histogram_(1, 10'000'000, 3)  // 1μs to 10s range, 3 significant digits
    , window_start_(std::chrono::steady_clock::now())
{}

void MetricsCollector::record_latency(int64_t latency_us) {
    histogram_.record(latency_us);
}

void MetricsCollector::record_transaction() {
    ++window_transactions_;
}

void MetricsCollector::record_error() {
    ++window_errors_;
}

void MetricsCollector::record_timeout() {
    ++window_timeouts_;
}

MetricsSnapshot MetricsCollector::snapshot_and_reset() {
    auto now = std::chrono::steady_clock::now();

    MetricsSnapshot snap;
    snap.hist = histogram_;                 // value copy (race-free: own thread)
    snap.txns = window_transactions_;
    snap.errors = window_errors_;
    snap.timeouts = window_timeouts_;
    snap.seconds = std::chrono::duration<double>(now - window_start_).count();

    // Reset for the next window.
    histogram_.reset();
    window_transactions_ = 0;
    window_errors_ = 0;
    window_timeouts_ = 0;
    window_start_ = now;

    return snap;
}

} // namespace bot_fleet::metrics
