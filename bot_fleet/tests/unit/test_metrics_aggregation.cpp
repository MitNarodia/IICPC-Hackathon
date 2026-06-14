/// tests/unit/test_metrics_aggregation.cpp
/// =======================================
/// Correctness of the per-shard collector -> cross-shard aggregator pipeline.
///
/// WHY THIS MATTERS:
///   This is the data path that turns millions of per-order records into the
///   leaderboard numbers. Two properties must hold:
///     (1) MetricsCollector::snapshot_and_reset() captures the window exactly
///         and then zeroes itself (so windows don't double-count).
///     (2) MetricsAggregator sums counters and merges histograms correctly,
///         and flush() resets only the rolling window while the cumulative
///         (authoritative end-of-run) view keeps accumulating.

#include <gtest/gtest.h>

#include "metrics/metrics_collector.hpp"
#include "metrics/metrics_aggregator.hpp"

#include <iostream>
#include <sstream>

using namespace bot_fleet::metrics;

// A collector snapshot must capture the window and then fully reset.
TEST(MetricsCollector, SnapshotCapturesThenResets) {
    MetricsCollector c;
    for (int i = 0; i < 500; ++i) {
        c.record_latency(250);
        c.record_transaction();
    }
    c.record_error();
    c.record_timeout();

    auto s = c.snapshot_and_reset();
    EXPECT_EQ(s.txns, 500u);
    EXPECT_EQ(s.errors, 1u);
    EXPECT_EQ(s.timeouts, 1u);
    EXPECT_EQ(s.hist.total_count(), 500);
    EXPECT_GT(s.seconds, 0.0);

    // Immediately snapshotting again must yield an empty window.
    auto s2 = c.snapshot_and_reset();
    EXPECT_EQ(s2.txns, 0u);
    EXPECT_EQ(s2.errors, 0u);
    EXPECT_EQ(s2.timeouts, 0u);
    EXPECT_EQ(s2.hist.total_count(), 0);
}

// Cumulative view must sum counters and merge latency across submissions.
TEST(MetricsAggregator, CumulativeSumsAcrossSubmissions) {
    MetricsAggregator agg(2);

    MetricsSnapshot s1;
    for (int i = 0; i < 300; ++i) { s1.hist.record(200); ++s1.txns; }
    s1.errors = 2;
    s1.seconds = 5.0;

    MetricsSnapshot s2;
    for (int i = 0; i < 700; ++i) { s2.hist.record(200); ++s2.txns; }
    s2.timeouts = 3;
    s2.seconds = 5.0;

    agg.submit(std::move(s1));
    agg.submit(std::move(s2));

    auto cum = agg.cumulative();
    EXPECT_EQ(cum.txns, 1000u);
    EXPECT_EQ(cum.errors, 2u);
    EXPECT_EQ(cum.timeouts, 3u);
    EXPECT_NEAR(static_cast<double>(cum.p50), 200.0, 4.0);
}

// flush() resets the rolling window but must NOT touch the cumulative view.
TEST(MetricsAggregator, FlushResetsWindowNotCumulative) {
    MetricsAggregator agg(1);

    MetricsSnapshot s;
    for (int i = 0; i < 100; ++i) { s.hist.record(150); ++s.txns; }
    s.seconds = 5.0;
    agg.submit(std::move(s));

    EXPECT_EQ(agg.window().txns, 100u);

    // flush() prints a human-readable line; redirect std::cout to keep test
    // output clean (and to prove flush() does not throw).
    std::ostringstream sink;
    auto* old = std::cout.rdbuf(sink.rdbuf());
    agg.flush();
    std::cout.rdbuf(old);

    EXPECT_EQ(agg.window().txns, 0u);       // rolling window cleared
    EXPECT_EQ(agg.cumulative().txns, 100u); // cumulative retained
}
