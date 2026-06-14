/// tests/unit/test_order_generation.cpp
/// ====================================
/// Validates that a bot produces well-formed orders from its persona + market.
///
/// WHY THIS MATTERS:
///   generate_order() is the per-iteration hot path that defines the actual
///   bytes sent to the exchange. Malformed orders (out-of-range quantity, a
///   limit order without a price, a corrupted side) would either crash a real
///   matching engine or silently skew results. We assert structural validity,
///   that the order-type mix tracks the configured persona ratios, and that
///   generation is deterministic per bot_id (the basis for replayable runs).
///
/// A Bot needs a ConnectionPool/MarketSimulator/MetricsCollector by reference,
/// but generate_order() touches none of the network — so we construct an
/// unconnected pool on a bare io_context and never run it.

#include <gtest/gtest.h>

#include "bot/bot.hpp"
#include "market/market_simulator.hpp"
#include "metrics/metrics_collector.hpp"
#include "net/connection.hpp"
#include "support/test_access.hpp"

#include <boost/asio/io_context.hpp>

using namespace bot_fleet;
using bot_fleet::test_access::BotProbe;

namespace {
bot::BotConfig make_cfg(uint64_t id, double limit, double market, double cancel) {
    bot::BotConfig c;
    c.bot_id = id;
    c.mean_interval_ms = 10.0;
    c.limit_ratio = limit;
    c.market_ratio = market;
    c.cancel_ratio = cancel;
    c.price_aggressiveness = 1.0;
    c.min_qty = 5;
    c.max_qty = 50;
    return c;
}
} // namespace

// Every generated order must be structurally valid.
TEST(OrderGeneration, FieldsAreValid) {
    boost::asio::io_context ioc;
    net_layer::ConnectionPool pool(ioc, 1);   // constructed, never connected
    market::MarketSimulator mkt(11);
    metrics::MetricsCollector met;

    bot::Bot b(make_cfg(1, 0.6, 0.2, 0.2), pool, mkt, met);
    const auto state = mkt.snapshot();

    uint64_t last_id = 0;
    bool first = true;
    for (int i = 0; i < 5000; ++i) {
        auto o = BotProbe::generate_order(b, state);
        EXPECT_EQ(o.bot_id, 1u);
        EXPECT_GE(o.quantity, 5u);
        EXPECT_LE(o.quantity, 50u);
        EXPECT_TRUE(o.side == market::Side::Buy || o.side == market::Side::Sell);

        if (o.type == market::OrderType::Limit) {
            EXPECT_GT(o.price, 0.0) << "limit order must carry a positive price";
        } else {
            EXPECT_DOUBLE_EQ(o.price, 0.0) << "market/cancel order must have no price";
        }

        if (!first) EXPECT_GT(o.order_id, last_id) << "order_id must be monotonic";
        last_id = o.order_id;
        first = false;
    }
}

// Over many orders the type mix must approximate the configured ratios.
TEST(OrderGeneration, TypeMixApproximatesPersonaRatios) {
    boost::asio::io_context ioc;
    net_layer::ConnectionPool pool(ioc, 1);
    market::MarketSimulator mkt(11);
    metrics::MetricsCollector met;

    bot::Bot b(make_cfg(42, 0.6, 0.3, 0.1), pool, mkt, met);
    const auto state = mkt.snapshot();

    int lim = 0, mar = 0, can = 0;
    const int N = 20000;
    for (int i = 0; i < N; ++i) {
        switch (BotProbe::generate_order(b, state).type) {
            case market::OrderType::Limit:  ++lim; break;
            case market::OrderType::Market: ++mar; break;
            case market::OrderType::Cancel: ++can; break;
        }
    }
    EXPECT_NEAR(lim / static_cast<double>(N), 0.6, 0.03);
    EXPECT_NEAR(mar / static_cast<double>(N), 0.3, 0.03);
    EXPECT_NEAR(can / static_cast<double>(N), 0.1, 0.03);
}

// Two bots with the same id + same market input must emit identical orders.
TEST(OrderGeneration, DeterministicPerBotId) {
    boost::asio::io_context ioc;
    net_layer::ConnectionPool p1(ioc, 1);
    net_layer::ConnectionPool p2(ioc, 1);
    market::MarketSimulator m1(5);
    market::MarketSimulator m2(5);
    metrics::MetricsCollector mc1;
    metrics::MetricsCollector mc2;

    bot::Bot a(make_cfg(7, 0.5, 0.3, 0.2), p1, m1, mc1);
    bot::Bot b(make_cfg(7, 0.5, 0.3, 0.2), p2, m2, mc2);
    const auto s1 = m1.snapshot();
    const auto s2 = m2.snapshot();

    for (int i = 0; i < 1000; ++i) {
        auto oa = BotProbe::generate_order(a, s1);
        auto ob = BotProbe::generate_order(b, s2);
        EXPECT_EQ(oa.side, ob.side);
        EXPECT_EQ(oa.type, ob.type);
        EXPECT_EQ(oa.quantity, ob.quantity);
        EXPECT_EQ(oa.order_id, ob.order_id);
    }
}
