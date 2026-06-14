/// bot/bot.cpp
/// ===========
/// Implements the bot coroutine — the core hot loop of the load generator.
///
/// EXECUTION FLOW (per bot, per iteration):
///   1. co_await sleep(interval)       — models realistic inter-order timing
///   2. Read market state              — atomic load, no lock
///   3. Generate order                 — deterministic from RNG + market
///   4. co_await pool.acquire()        — get a WebSocket connection
///   5. Stamp send_time                — latency measurement start point
///   6. co_await conn->send(order)     — write to socket (async)
///   7. co_await conn->receive()       — read response (async)
///   8. Stamp recv_time                — latency measurement end point
///   9. Record latency & transaction   — metrics (no lock, same thread)
///  10. pool.release(conn)             — return connection to pool
///  11. Repeat
///
/// COROUTINE SUSPENSION POINTS (where other bots get to run):
///   Steps 1, 4, 6, 7 are co_await points. The event loop processes other
///   bots' completions during each suspension.

#include "bot/bot.hpp"
#include <boost/asio/steady_timer.hpp>
#include <boost/asio/use_awaitable.hpp>
#include <chrono>

namespace bot_fleet::bot {

Bot::Bot(BotConfig config,
         net_layer::ConnectionPool& pool,
         market::MarketSimulator& market,
         metrics::MetricsCollector& metrics)
    : config_(std::move(config))
    , pool_(pool)
    , market_(market)
    , metrics_(metrics)
    , rng_(config_.bot_id * 2654435761ULL)  // Knuth multiplicative hash for seed spread
    , interval_dist_(1.0 / config_.mean_interval_ms)
    , uniform_(0.0, 1.0)
    , qty_dist_(config_.min_qty, config_.max_qty)
{}

::net::awaitable<void> Bot::run(uint64_t max_orders) {
    auto executor = co_await ::net::this_coro::executor;

    // One timer for the bot's entire lifetime — reused every iteration instead
    // of constructing/destructing a steady_timer (and its internal timer-queue
    // node) on each order. At 100k bots this removes 100k allocations/cancels
    // per order round.
    ::net::steady_timer timer(executor);

    // Reserve once so the reusable send buffer never reallocates mid-run.
    send_buf_.reserve(160);

    for (uint64_t i = 0; i < max_orders; ++i) {
        // ── Step 1: Sleep (models inter-order arrival) ──────────────────
        timer.expires_after(next_interval());
        co_await timer.async_wait(::net::use_awaitable);

        // ── Step 2: Read market state (atomic, no lock) ─────────────────
        auto state = market_.snapshot();

        // ── Step 3: Generate order ──────────────────────────────────────
        auto order = generate_order(state);

        // ── Step 4: Acquire connection from pool ────────────────────────
        auto* conn = co_await pool_.acquire();

        try {
            // ── Step 5: Stamp send time ─────────────────────────────────
            order.send_time = market::Clock::now();

            // ── Step 6: Send order (serialize into reusable buffer) ──────
            market::serialize_order_into(send_buf_, order);
            co_await conn->send(send_buf_);

            // ── Step 7: Receive response ────────────────────────────────
            auto response_str = co_await conn->receive();

            // ── Step 8: Stamp receive time ──────────────────────────────
            auto recv_time = market::Clock::now();

            // ── Step 9: Record metrics ──────────────────────────────────
            auto latency_us = std::chrono::duration_cast<std::chrono::microseconds>(
                recv_time - order.send_time
            ).count();

            metrics_.record_latency(latency_us);
            metrics_.record_transaction();

        } catch (const std::exception&) {
            metrics_.record_error();
        }

        // ── Step 10: Release connection back to pool ────────────────────
        pool_.release(conn);
    }
}

market::Order Bot::generate_order(const market::MarketState& state) {
    market::Order order{};
    order.bot_id = config_.bot_id;
    order.order_id = next_order_id_++;

    // Determine side: 50/50 buy/sell
    order.side = uniform_(rng_) < 0.5 ? market::Side::Buy : market::Side::Sell;

    // Determine order type based on persona mix
    double r = uniform_(rng_);
    if (r < config_.limit_ratio) {
        order.type = market::OrderType::Limit;
    } else if (r < config_.limit_ratio + config_.market_ratio) {
        order.type = market::OrderType::Market;
    } else {
        order.type = market::OrderType::Cancel;
    }

    // Set price relative to mid (for limit orders)
    if (order.type == market::OrderType::Limit) {
        double offset = state.spread * config_.price_aggressiveness * (uniform_(rng_) - 0.5);
        order.price = state.mid_price + offset;
    } else {
        order.price = 0.0;  // Not relevant for market/cancel
    }

    // Set quantity
    order.quantity = qty_dist_(rng_);

    return order;
}

std::chrono::milliseconds Bot::next_interval() {
    // Exponential distribution models Poisson arrival process (realistic)
    double ms = interval_dist_(rng_);
    // Clamp to [1ms, 1000ms] to avoid degenerate values
    ms = std::clamp(ms, 1.0, 1000.0);
    return std::chrono::milliseconds(static_cast<int64_t>(ms));
}

} // namespace bot_fleet::bot
