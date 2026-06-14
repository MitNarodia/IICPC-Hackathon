/// tests/unit/test_seqlock.cpp
/// ===========================
/// SINGLE-THREADED correctness of the SeqLock-backed MarketSimulator.
///
/// WHY THIS MATTERS:
///   Before any concurrency is involved we must prove the basic read/write
///   protocol is sound: a fresh simulator publishes the documented defaults,
///   each tick() advances the sequence so snapshot() observes a *complete*,
///   self-consistent state, and the tick counter tracks the number of writes.
///   If these single-threaded properties are wrong, the multi-threaded
///   torn-read test results would be meaningless.

#include <gtest/gtest.h>

#include "market/market_simulator.hpp"
#include "support/market_invariants.hpp"

using bot_fleet::market::MarketSimulator;
using bot_fleet::test_support::is_consistent;

// A brand-new simulator must expose the documented initial market state.
TEST(SeqLockUnit, InitialSnapshotIsDefault) {
    MarketSimulator sim(42);
    auto s = sim.snapshot();
    EXPECT_DOUBLE_EQ(s.mid_price, 100.0);
    EXPECT_DOUBLE_EQ(s.spread, 0.02);
    EXPECT_DOUBLE_EQ(s.volatility, 0.01);
    EXPECT_FALSE(s.is_volatile);
    EXPECT_EQ(sim.tick_count(), 0u);
}

// One tick advances the write counter and still yields a consistent snapshot.
TEST(SeqLockUnit, TickAdvancesAndStaysConsistent) {
    MarketSimulator sim(7);
    sim.tick();
    EXPECT_EQ(sim.tick_count(), 1u);
    EXPECT_TRUE(is_consistent(sim.snapshot()));
}

// Across many sequential writes, every reader-visible state is well-formed
// (the seq is always even at snapshot time → no partially-applied update).
TEST(SeqLockUnit, InvariantHoldsAcrossManyTicks) {
    MarketSimulator sim(12345);
    constexpr int kTicks = 200000;
    for (int i = 0; i < kTicks; ++i) {
        sim.tick();
        ASSERT_TRUE(is_consistent(sim.snapshot())) << "inconsistent at tick " << i;
    }
    EXPECT_EQ(sim.tick_count(), static_cast<uint64_t>(kTicks));
}
