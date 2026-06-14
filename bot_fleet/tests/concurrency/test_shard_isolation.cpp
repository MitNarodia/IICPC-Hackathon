/// tests/concurrency/test_shard_isolation.cpp
/// ==========================================
/// Proves the share-nothing sharding assigns work cleanly and independently.
///
/// WHY THIS MATTERS:
///   Thread-per-core correctness rests on three guarantees:
///     (1) The strided assignment (worker w owns ids w, w+W, w+2W, ...)
///         partitions [0, N) EXACTLY — every bot runs on exactly one shard,
///         none dropped, none duplicated. A bug here would silently under- or
///         double-count load.
///     (2) The worker count is capped to the bot count, so no shard is created
///         empty (an empty shard would never cancel its timers -> hang; this is
///         the same root cause guarded by the clean-shutdown test).
///     (3) A bot's persona is generated centrally and is INDEPENDENT of how
///         many workers run it, so results are comparable across --workers
///         values and runs are reproducible.

#include <gtest/gtest.h>

#include "bot/bot_coordinator.hpp"
#include "support/test_access.hpp"

#include <vector>

using bot_fleet::bot::BotCoordinator;
using bot_fleet::bot::RunConfig;
using bot_fleet::test_access::CoordinatorProbe;

namespace {
// Mirrors run_worker()'s loop `for (id = w; id < N; id += W)`. Asserts the
// union over all workers is a perfect partition of [0, N).
void expect_exact_partition(uint32_t N, unsigned W) {
    std::vector<int> owner(N, -1);
    for (unsigned w = 0; w < W; ++w) {
        for (uint32_t id = w; id < N; id += W) {
            ASSERT_EQ(owner[id], -1) << "bot id " << id << " assigned twice";
            owner[id] = static_cast<int>(w);
        }
    }
    for (uint32_t id = 0; id < N; ++id) {
        ASSERT_NE(owner[id], -1) << "bot id " << id << " never assigned";
    }
}
} // namespace

// The strided partition must be exact for divisible, indivisible, and
// fewer-bots-than-workers cases.
TEST(ShardIsolation, StridingPartitionsBotsExactly) {
    expect_exact_partition(1000, 8);
    expect_exact_partition(10000, 7);   // not divisible
    expect_exact_partition(5, 8);       // more workers than bots (pre-cap math)
    expect_exact_partition(1, 1);
}

// The constructor must cap workers to the bot count and honour explicit counts.
TEST(ShardIsolation, WorkerCountCappedToBotCount) {
    {
        RunConfig cfg; cfg.num_bots = 2; cfg.num_workers = 64;
        BotCoordinator c(cfg);
        EXPECT_EQ(CoordinatorProbe::num_workers(c), 2u) << "must cap to bot count";
    }
    {
        RunConfig cfg; cfg.num_bots = 1000; cfg.num_workers = 4;
        BotCoordinator c(cfg);
        EXPECT_EQ(CoordinatorProbe::num_workers(c), 4u) << "must honour explicit count";
    }
    {
        RunConfig cfg; cfg.num_bots = 1000; cfg.num_workers = 0;  // auto
        BotCoordinator c(cfg);
        unsigned w = CoordinatorProbe::num_workers(c);
        EXPECT_GE(w, 1u);
        EXPECT_LE(w, 1000u);
    }
}

// A bot's identity/persona must not depend on the worker count.
TEST(ShardIsolation, ConfigsIndependentOfWorkerCount) {
    RunConfig a; a.num_bots = 3000; a.run_seed = 2024; a.num_workers = 1;
    RunConfig b = a; b.num_workers = 16;

    BotCoordinator ca(a);
    BotCoordinator cb(b);
    auto va = CoordinatorProbe::generate_bot_configs(ca);
    auto vb = CoordinatorProbe::generate_bot_configs(cb);

    ASSERT_EQ(va.size(), vb.size());
    for (size_t i = 0; i < va.size(); ++i) {
        EXPECT_EQ(va[i].bot_id, vb[i].bot_id);
        EXPECT_DOUBLE_EQ(va[i].price_aggressiveness, vb[i].price_aggressiveness);
        EXPECT_DOUBLE_EQ(va[i].mean_interval_ms, vb[i].mean_interval_ms);
    }
}
