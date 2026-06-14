/// tests/concurrency/test_seqlock_torn_reads.cpp
/// =============================================
/// THE core concurrency guarantee of the shared market state.
///
/// WHY THIS MATTERS:
///   After the thread-per-core redesign, ONE writer (worker 0) ticks the
///   MarketSimulator while EVERY other worker thread reads it on its hot path.
///   The MarketState is 25 bytes (three doubles + a bool) — far larger than any
///   atomic the hardware can publish in one instruction. If the SeqLock were
///   wrong, readers would observe a half-written state: a new `is_volatile`
///   flag paired with an old `volatility`, or a torn 8-byte `mid_price`.
///
///   This test runs the exact production pattern (1 writer, N readers) under
///   heavy contention and checks the coupling invariant on EVERY read. Zero
///   violations across tens of millions of reads is the evidence that the
///   SeqLock publishes atomically. Run it under ThreadSanitizer as well to
///   prove there is no data race on the sequence/payload.

#include <gtest/gtest.h>

#include "market/market_simulator.hpp"
#include "support/market_invariants.hpp"

#include <atomic>
#include <chrono>
#include <thread>
#include <vector>

using bot_fleet::market::MarketSimulator;
using bot_fleet::test_support::is_consistent;

TEST(SeqLockConcurrency, NoTornReadsUnderContention) {
    MarketSimulator sim(31337);

    std::atomic<bool> stop{false};
    std::atomic<uint64_t> violations{0};
    std::atomic<uint64_t> total_reads{0};

    // Single writer — matches the production invariant that only worker 0
    // ever calls tick(). Two writers would corrupt the sequence.
    std::thread writer([&] {
        while (!stop.load(std::memory_order_relaxed)) {
            sim.tick();
        }
    });

    // Many concurrent readers, each validating every snapshot it observes.
    constexpr int kReaders = 8;
    std::vector<std::thread> readers;
    readers.reserve(kReaders);
    for (int i = 0; i < kReaders; ++i) {
        readers.emplace_back([&] {
            uint64_t local = 0;
            while (!stop.load(std::memory_order_relaxed)) {
                auto s = sim.snapshot();
                if (!is_consistent(s)) {
                    violations.fetch_add(1, std::memory_order_relaxed);
                }
                ++local;
            }
            total_reads.fetch_add(local, std::memory_order_relaxed);
        });
    }

    std::this_thread::sleep_for(std::chrono::milliseconds(750));
    stop.store(true, std::memory_order_relaxed);

    writer.join();
    for (auto& t : readers) t.join();

    EXPECT_EQ(violations.load(), 0u) << "SeqLock permitted a torn/partial read";
    EXPECT_GT(total_reads.load(), 0u) << "readers never observed any state";
    EXPECT_GT(sim.tick_count(), 0u) << "writer never advanced";
}
