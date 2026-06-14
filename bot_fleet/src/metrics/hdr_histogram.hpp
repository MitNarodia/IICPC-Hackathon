#pragma once

/// metrics/hdr_histogram.hpp
/// =========================
/// WHY THIS FILE EXISTS:
///   Accurate percentile measurement (p50, p90, p99) requires a data structure
///   that doesn't degrade under high insertion rate and uses bounded memory.
///   A sorted vector would require O(n log n) or O(n) memory per window.
///   HDR Histogram provides O(1) record, O(1) memory (fixed-size array),
///   and accurate percentiles with configurable significant digits.
///
/// CLASS: HdrHistogram
///   A logarithmic-linear bucket histogram inspired by Gil Tene's HdrHistogram.
///   Stores counts in pre-allocated buckets covering [min_value, max_value]
///   with a configurable number of significant figures of precision.
///
///   For this MVP: range [1 μs, 10 s], 3 significant digits.
///   Total bucket count ≈ 3,968 entries × 8 bytes = ~32 KB per histogram.
///
/// THREADING:
///   NOT thread-safe. Each thread/io_context owns its own histogram instance.
///   Periodic merge into a global histogram happens on the metrics thread.
///
/// QUEUES: None. Direct array access.
///
/// ASYNC OPS: None. Pure computation.

#include <cstdint>
#include <vector>
#include <cmath>
#include <algorithm>
#include <numeric>

namespace bot_fleet::metrics {

class HdrHistogram {
public:
    /// Construct histogram covering [min_value_us, max_value_us] microseconds.
    /// significant_digits: 1-5, controls bucket granularity.
    explicit HdrHistogram(
        int64_t min_value_us = 1,
        int64_t max_value_us = 10'000'000,  // 10 seconds in μs
        int significant_digits = 3
    );

    /// Record a latency sample in microseconds. O(1).
    void record(int64_t value_us);

    /// Get the value at a given percentile (0.0 - 100.0).
    int64_t percentile(double p) const;

    /// Total number of recorded samples.
    int64_t total_count() const { return total_count_; }

    /// Arithmetic mean (for informational purposes; percentiles are primary).
    double mean() const;

    /// Reset all counts. Used at window boundaries.
    void reset();

    /// Merge another histogram's counts into this one.
    void merge(const HdrHistogram& other);

private:
    int bucket_index(int64_t value) const;
    int64_t value_at_index(int index) const;

    int64_t min_value_;
    int64_t max_value_;
    int significant_digits_;
    int sub_bucket_count_;       // Number of sub-buckets per power-of-2 range
    int bucket_count_;           // Number of power-of-2 ranges
    int total_buckets_;          // Total array size

    std::vector<int64_t> counts_;
    int64_t total_count_ = 0;
    int64_t total_sum_ = 0;      // For mean calculation
};

} // namespace bot_fleet::metrics
