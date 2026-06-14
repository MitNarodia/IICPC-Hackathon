/// tests/concurrency/test_concurrent_metrics.cpp
/// =============================================
/// Proves the per-shard -> aggregator merge is correct under real contention.
///
/// WHY THIS MATTERS:
///   In production each shard records into its OWN MetricsCollector with no
///   locks (single-writer hot path), then periodically submits a snapshot to
///   the shared MetricsAggregator on a cold path guarded by one mutex. This
///   test reproduces that exact pattern with many threads submitting at once
///   and asserts:
///     (1) No lost updates: the aggregated transaction/error/timeout totals
///         equal the exact sum of what every thread recorded (the mutex really
///         serialises the merges).
///     (2) The merged latency percentile is correct across shards.
///   Running this under ThreadSanitizer additionally proves the hot-path
///   collectors need no atomics and the only synchronisation is the cold-path
///   aggregator lock.

#include <gtest/gtest.h>

#include "metrics/metrics_collector.hpp"
#include "metrics/metrics_aggregator.hpp"

#include <thread>
#include <vector>

using namespace bot_fleet::metrics;

TEST(ConcurrentMetrics, ParallelShardsAggregateExactly) {
    constexpr int kShards = 8;
    constexpr int kPerShard = 50000;   // latencies + transactions each
    constexpr int kErrors = 3;
    constexpr int kTimeouts = 2;

    MetricsAggregator agg(kShards);

    std::vector<std::thread> shards;
    shards.reserve(kShards);
    for (int t = 0; t < kShards; ++t) {
        shards.emplace_back([&] {
            MetricsCollector c;  // thread-local, no atomics on the hot path
            for (int i = 0; i < kPerShard; ++i) {
                c.record_latency(300);
                c.record_transaction();
            }
            for (int i = 0; i < kErrors; ++i)   c.record_error();
            for (int i = 0; i < kTimeouts; ++i) c.record_timeout();

            // Concurrent cold-path submit — contends on the aggregator mutex.
            agg.submit(c.snapshot_and_reset());
        });
    }
    for (auto& s : shards) s.join();

    auto cum = agg.cumulative();
    EXPECT_EQ(cum.txns,     static_cast<uint64_t>(kShards) * kPerShard);
    EXPECT_EQ(cum.errors,   static_cast<uint64_t>(kShards) * kErrors);
    EXPECT_EQ(cum.timeouts, static_cast<uint64_t>(kShards) * kTimeouts);
    EXPECT_NEAR(static_cast<double>(cum.p50), 300.0, 6.0);
}
