#pragma once

/// tests/support/test_access.hpp
/// =============================
/// Test-only "probe" classes that are declared as friends inside the
/// production headers (via a forward declaration only). This lets the unit
/// tests exercise otherwise-private *pure logic* — order generation, persona
/// generation, worker-count capping — WITHOUT widening the production API with
/// getters that exist solely for testing.
///
/// The core library never includes this file; it only sees the forward-declared
/// names `bot_fleet::test_access::BotProbe` / `CoordinatorProbe`. The actual
/// class bodies below are compiled only into the test binary.

#include "bot/bot.hpp"
#include "bot/bot_coordinator.hpp"
#include "market/order.hpp"
#include "market/market_simulator.hpp"

#include <chrono>
#include <vector>

namespace bot_fleet::test_access {

/// Grants tests access to Bot's private generation helpers.
class BotProbe {
public:
    static market::Order generate_order(bot::Bot& b, const market::MarketState& s) {
        return b.generate_order(s);
    }
    static std::chrono::milliseconds next_interval(bot::Bot& b) {
        return b.next_interval();
    }
};

/// Grants tests access to BotCoordinator's private config generator and the
/// constructor-computed worker count (which is capped to the bot count).
class CoordinatorProbe {
public:
    static std::vector<bot::BotConfig> generate_bot_configs(bot::BotCoordinator& c) {
        return c.generate_bot_configs();
    }
    static unsigned num_workers(const bot::BotCoordinator& c) {
        return c.num_workers_;
    }
};

} // namespace bot_fleet::test_access
