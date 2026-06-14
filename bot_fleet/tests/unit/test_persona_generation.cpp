/// tests/unit/test_persona_generation.cpp
/// ======================================
/// Validates the fleet's participant-mix generator.
///
/// WHY THIS MATTERS:
///   The realism of the load test depends on the population of bot personas
///   matching the intended market-maker / momentum / noise / passive mix
///   (~10% / 20% / 30% / 40%). If the distribution drifts, the generated load
///   no longer resembles a real order book and benchmark numbers lose meaning.
///   Generation must also be deterministic for a fixed seed (reproducibility)
///   and assign contiguous bot ids (so the strided sharding covers them all).
///
/// Personas are recovered from each config by their unique price_aggressiveness
/// fingerprint set in BotCoordinator::generate_bot_configs():
///     0.5 = market maker, 2.0 = momentum, 1.5 = noise, 3.0 = passive.

#include <gtest/gtest.h>

#include "bot/bot_coordinator.hpp"
#include "support/test_access.hpp"

#include <map>

using bot_fleet::bot::BotCoordinator;
using bot_fleet::bot::RunConfig;
using bot_fleet::test_access::CoordinatorProbe;

namespace {
enum Persona { MM, MOM, NOISE, PASS, UNKNOWN };

Persona classify(double aggressiveness) {
    if (aggressiveness == 0.5) return MM;
    if (aggressiveness == 2.0) return MOM;
    if (aggressiveness == 1.5) return NOISE;
    if (aggressiveness == 3.0) return PASS;
    return UNKNOWN;
}
} // namespace

// The empirical persona mix over a large population must match the design.
TEST(PersonaGeneration, DistributionMatchesDesignMix) {
    RunConfig cfg;
    cfg.num_bots = 10000;
    cfg.run_seed = 12345;

    BotCoordinator coord(cfg);
    auto configs = CoordinatorProbe::generate_bot_configs(coord);
    ASSERT_EQ(configs.size(), 10000u);

    std::map<int, int> counts;
    for (const auto& c : configs) counts[classify(c.price_aggressiveness)]++;

    EXPECT_EQ(counts[UNKNOWN], 0) << "a config had an unrecognised persona";

    const double n = 10000.0;
    // 3% absolute tolerance: with N=10000 the 3-sigma sampling band is < 1.5%.
    EXPECT_NEAR(counts[MM]    / n, 0.10, 0.03);
    EXPECT_NEAR(counts[MOM]   / n, 0.20, 0.03);
    EXPECT_NEAR(counts[NOISE] / n, 0.30, 0.03);
    EXPECT_NEAR(counts[PASS]  / n, 0.40, 0.03);
}

// Same seed => identical persona assignment (reproducible load).
TEST(PersonaGeneration, DeterministicForSameSeed) {
    RunConfig cfg;
    cfg.num_bots = 2000;
    cfg.run_seed = 777;

    BotCoordinator c1(cfg);
    BotCoordinator c2(cfg);
    auto a = CoordinatorProbe::generate_bot_configs(c1);
    auto b = CoordinatorProbe::generate_bot_configs(c2);

    ASSERT_EQ(a.size(), b.size());
    for (size_t i = 0; i < a.size(); ++i) {
        EXPECT_EQ(a[i].bot_id, b[i].bot_id);
        EXPECT_DOUBLE_EQ(a[i].price_aggressiveness, b[i].price_aggressiveness);
        EXPECT_DOUBLE_EQ(a[i].mean_interval_ms, b[i].mean_interval_ms);
        EXPECT_EQ(a[i].max_qty, b[i].max_qty);
    }
}

// Bot ids must be 0..N-1 so the strided shard assignment covers every bot.
TEST(PersonaGeneration, BotIdsAreContiguous) {
    RunConfig cfg;
    cfg.num_bots = 500;
    cfg.run_seed = 1;

    BotCoordinator c(cfg);
    auto v = CoordinatorProbe::generate_bot_configs(c);
    ASSERT_EQ(v.size(), 500u);
    for (uint32_t i = 0; i < v.size(); ++i) {
        EXPECT_EQ(v[i].bot_id, i);
    }
}
