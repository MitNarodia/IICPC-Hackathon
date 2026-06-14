/// tests/unit/test_hdr_histogram.cpp
/// =================================
/// Correctness of the latency histogram that produces p50/p90/p99.
///
/// WHY THIS MATTERS:
///   Every latency number the fleet reports comes out of this structure, and
///   after the thread-per-core redesign the authoritative percentiles are
///   produced by MERGING per-shard histograms. So we must prove two things:
///     (1) record/percentile/mean/reset behave correctly within the bucket
///         precision, and
///     (2) merge() is exactly additive — a merged histogram is byte-for-byte
///         indistinguishable (at the percentile level) from one that recorded
///         every sample directly. That additive property is the mathematical
///         justification for sharding metrics across cores.

#include <gtest/gtest.h>

#include "metrics/hdr_histogram.hpp"

using bot_fleet::metrics::HdrHistogram;

// A constant stream should report that constant at every percentile, within
// the histogram's 3-significant-digit bucket precision (~0.2%, we allow 2%).
TEST(HdrHistogram, RecordsConstantAccurately) {
    HdrHistogram h(1, 10'000'000, 3);
    for (int i = 0; i < 1000; ++i) h.record(100);

    EXPECT_EQ(h.total_count(), 1000);
    EXPECT_NEAR(static_cast<double>(h.percentile(50)), 100.0, 2.0);
    EXPECT_NEAR(h.mean(), 100.0, 2.0);
}

// Percentiles must be monotonically non-decreasing and land near the true
// quantiles of a uniform 1..1000 spread.
TEST(HdrHistogram, PercentilesMonotonicAndAccurate) {
    HdrHistogram h(1, 10'000'000, 3);
    for (int v = 1; v <= 1000; ++v) h.record(v);

    const auto p50 = h.percentile(50);
    const auto p90 = h.percentile(90);
    const auto p99 = h.percentile(99);

    EXPECT_LE(p50, p90);
    EXPECT_LE(p90, p99);
    EXPECT_NEAR(static_cast<double>(p50), 500.0, 500.0 * 0.05);
    EXPECT_NEAR(static_cast<double>(p90), 900.0, 900.0 * 0.05);
    EXPECT_NEAR(static_cast<double>(p99), 990.0, 990.0 * 0.05);
}

// reset() must zero all state so a recycled window starts clean.
TEST(HdrHistogram, ResetClearsAllState) {
    HdrHistogram h;
    for (int i = 0; i < 10; ++i) h.record(123);
    h.reset();
    EXPECT_EQ(h.total_count(), 0);
    EXPECT_EQ(h.percentile(50), 0);
    EXPECT_DOUBLE_EQ(h.mean(), 0.0);
}

// Out-of-range samples are clamped into [min,max] rather than dropped/crashing.
TEST(HdrHistogram, ClampsOutOfRangeValues) {
    HdrHistogram h(1, 1000, 3);
    h.record(-5);          // below min -> clamps to 1
    h.record(1'000'000);   // above max -> clamps to 1000
    EXPECT_EQ(h.total_count(), 2);
    EXPECT_GE(h.percentile(99), 1);
}

// THE KEY SHARDING-CORRECTNESS TEST: merge two shard histograms and compare
// against a reference that recorded the union of samples directly. Because the
// bucket layout is shared and merge is additive, the percentiles must be EXACT
// (not merely close).
TEST(HdrHistogram, MergeEqualsDirectRecording) {
    HdrHistogram a(1, 10'000'000, 3);
    HdrHistogram b(1, 10'000'000, 3);
    HdrHistogram ref(1, 10'000'000, 3);

    for (int i = 0; i < 1000; ++i) { a.record(100); ref.record(100); }
    for (int i = 0; i < 1000; ++i) { b.record(800); ref.record(800); }

    a.merge(b);

    EXPECT_EQ(a.total_count(), ref.total_count());
    for (double p : {1.0, 10.0, 25.0, 50.0, 75.0, 90.0, 99.0, 99.9}) {
        EXPECT_EQ(a.percentile(p), ref.percentile(p)) << "mismatch at p=" << p;
    }
    EXPECT_DOUBLE_EQ(a.mean(), ref.mean());
}
