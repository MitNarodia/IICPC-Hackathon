/// tests/unit/test_market_simulator.cpp
/// ====================================
/// Behavioural correctness of the synthetic market model.
///
/// WHY THIS MATTERS:
///   The simulator drives every bot's order prices and the regime-dependent
///   bursts that make the load realistic. Bots are seeded deterministically,
///   so replayable, bug-for-bug-identical runs depend on the simulator being
///   deterministic for a fixed seed. We also guard the two numeric safety
///   properties (price floor + finiteness) and prove the regime machine
///   actually switches between calm and volatile states.

#include <gtest/gtest.h>

#include "market/market_simulator.hpp"
#include "support/market_invariants.hpp"

#include <cmath>

using bot_fleet::market::MarketSimulator;
using bot_fleet::test_support::is_consistent;

// Same seed => identical trajectory (required for reproducible benchmarks).
TEST(MarketSimulator, DeterministicForSameSeed) {
    MarketSimulator a(999);
    MarketSimulator b(999);
    for (int i = 0; i < 10000; ++i) { a.tick(); b.tick(); }

    auto sa = a.snapshot();
    auto sb = b.snapshot();
    EXPECT_DOUBLE_EQ(sa.mid_price, sb.mid_price);
    EXPECT_DOUBLE_EQ(sa.volatility, sb.volatility);
    EXPECT_EQ(sa.is_volatile, sb.is_volatile);
}

// Different seeds => diverging trajectories (the RNG actually drives state).
TEST(MarketSimulator, DifferentSeedsDiverge) {
    MarketSimulator a(1);
    MarketSimulator b(2);
    for (int i = 0; i < 10000; ++i) { a.tick(); b.tick(); }
    EXPECT_NE(a.snapshot().mid_price, b.snapshot().mid_price);
}

// Price must never go non-finite or below the 1.0 floor, even over long runs.
TEST(MarketSimulator, PriceStaysFiniteAboveFloor) {
    MarketSimulator sim(2026);
    for (int i = 0; i < 500000; ++i) {
        sim.tick();
        auto s = sim.snapshot();
        ASSERT_TRUE(std::isfinite(s.mid_price));
        ASSERT_GE(s.mid_price, 1.0);
    }
}

// Over many ticks the regime machine must visit BOTH states, and every visited
// state must keep (is_volatile, volatility, spread) mutually consistent.
TEST(MarketSimulator, RegimeSwitchingOccursAndStaysCoupled) {
    MarketSimulator sim(2027);
    bool seen_calm = false;
    bool seen_volatile = false;

    for (int i = 0; i < 200000; ++i) {
        sim.tick();
        auto s = sim.snapshot();
        ASSERT_TRUE(is_consistent(s));
        seen_calm     |= !s.is_volatile;
        seen_volatile |=  s.is_volatile;
    }

    EXPECT_TRUE(seen_calm);
    EXPECT_TRUE(seen_volatile) << "no volatile regime seen across 200k ticks";
}
