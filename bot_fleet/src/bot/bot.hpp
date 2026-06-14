#pragma once

/// bot/bot.hpp
/// ===========
/// WHY THIS FILE EXISTS:
///   Defines the Bot class — the fundamental unit of load generation.
///   Each bot is a persona-driven trading agent that runs as a coroutine.
///   1000 bots = 1000 coroutines, all multiplexed onto a single thread
///   via the io_context event loop. This is the "no thread-per-bot" design.
///
/// CLASSES:
///   BotConfig — Static configuration for a bot's persona (strategy, timing).
///   Bot — The coroutine-based trading agent.
///
/// THREADING MODEL:
///   Each Bot::run() is a coroutine co_spawned onto the io_context.
///   All 1000 bots share one thread. When a bot co_awaits (sleep, send, recv),
///   it suspends and the event loop processes other bots' completions.
///   NO MUTEXES in the hot path.
///
/// QUEUES:
///   Bots do not use explicit queues. They interact with:
///   - ConnectionPool (acquire/release pattern)
///   - MetricsCollector (direct record calls, no queue — same thread)
///   - MarketSimulator (atomic read of shared state)
///
/// ASYNC OPERATIONS:
///   - co_await timer expiry (inter-order delay based on persona)
///   - co_await pool.acquire() (wait for available connection)
///   - co_await conn->send() (write order to socket)
///   - co_await conn->receive() (read response from socket)

#include "market/order.hpp"
#include "market/market_simulator.hpp"
#include "metrics/metrics_collector.hpp"
#include "net/connection.hpp"

#include <boost/asio/awaitable.hpp>
#include <cstdint>
#include <random>

// Forward-declared test shim. Declared a friend below so unit tests can drive
// the bot's private generation logic without adding public getters. The class
// is ONLY ever defined in the test binary; the core library sees just this name.
namespace bot_fleet::test_access { class BotProbe; }

namespace bot_fleet::bot {

/// Persona configuration — determines how aggressive/passive a bot is.
struct BotConfig {
    uint64_t bot_id;

    // Timing: mean inter-order interval in milliseconds.
    // Market makers: 1-5ms. Noise traders: 50-500ms.
    double mean_interval_ms = 10.0;

    // Order mix probabilities (must sum to 1.0)
    double limit_ratio  = 0.6;
    double market_ratio = 0.2;
    double cancel_ratio = 0.2;

    // Price offset from mid (in spread multiples)
    double price_aggressiveness = 1.0;

    // Quantity range
    uint32_t min_qty = 1;
    uint32_t max_qty = 100;
};

/// A single trading bot — runs as a coroutine for its entire lifetime.
class Bot {
    // Test-only access to private generation logic (see test_access shim).
    friend class ::bot_fleet::test_access::BotProbe;

public:
    Bot(BotConfig config,
        net_layer::ConnectionPool& pool,
        market::MarketSimulator& market,
        metrics::MetricsCollector& metrics);

    /// The main coroutine loop. co_spawn this onto the io_context.
    /// Runs until cancelled or max_orders reached.
    ::net::awaitable<void> run(uint64_t max_orders);

private:
    /// Generate the next order based on persona + market conditions.
    market::Order generate_order(const market::MarketState& state);

    /// Compute sleep duration based on persona (exponential distribution).
    std::chrono::milliseconds next_interval();

    BotConfig config_;
    net_layer::ConnectionPool& pool_;
    market::MarketSimulator& market_;
    metrics::MetricsCollector& metrics_;

    // Per-bot deterministic RNG (seeded from bot_id for replay)
    std::mt19937_64 rng_;
    std::exponential_distribution<double> interval_dist_;
    std::uniform_real_distribution<double> uniform_;
    std::uniform_int_distribution<uint32_t> qty_dist_;

    // Reusable serialization buffer — avoids a heap allocation per order on
    // the send path. Lives for the bot's whole lifetime.
    std::string send_buf_;

    uint64_t next_order_id_ = 0;
};

} // namespace bot_fleet::bot
