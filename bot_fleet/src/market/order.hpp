#pragma once

/// market/order.hpp
/// ================
/// WHY THIS FILE EXISTS:
///   Defines the vocabulary types for the trading domain. Every component
///   (bots, connection manager, metrics) speaks in terms of these types.
///   Centralizing them prevents circular dependencies and ensures a single
///   source of truth for message schemas.

#include <cstdint>
#include <string>
#include <chrono>

namespace bot_fleet::market {

/// Monotonic clock used for all latency measurements.
/// steady_clock is immune to NTP adjustments — critical for accurate deltas.
using Clock = std::chrono::steady_clock;
using Timestamp = Clock::time_point;
using Duration = Clock::duration;

/// Order side: buy or sell.
enum class Side : uint8_t {
    Buy  = 0,
    Sell = 1
};

/// Order type determines matching semantics.
enum class OrderType : uint8_t {
    Limit  = 0,
    Market = 1,
    Cancel = 2
};

/// Outbound order from a bot to the contestant's exchange.
/// Kept small and trivially copyable for cache-friendly storage.
struct Order {
    uint64_t  bot_id;          // Which bot generated this
    uint64_t  order_id;        // Unique per-bot monotonic sequence
    Side      side;
    OrderType type;
    double    price;           // Ignored for Market orders
    uint32_t  quantity;
    Timestamp send_time;       // Stamped just before write to socket
};

/// Inbound response from the contestant's exchange.
enum class ResponseStatus : uint8_t {
    Accepted = 0,
    Rejected = 1,
    Filled   = 2,
    Cancelled = 3,
    Error    = 4
};

struct Response {
    uint64_t       order_id;
    ResponseStatus status;
    Timestamp      recv_time;  // Stamped at first byte parsed
};

/// Serializes an order into a caller-owned buffer, avoiding a per-order heap
/// allocation. The bot hot loop holds one reusable buffer and calls this on
/// every iteration, so at high message rates we never touch the global
/// allocator on the send path. `out` is cleared and overwritten.
inline void serialize_order_into(std::string& out, const Order& o) {
    out.clear();
    out += R"({"bot_id":)";
    out += std::to_string(o.bot_id);
    out += R"(,"order_id":)";
    out += std::to_string(o.order_id);
    out += R"(,"side":)";
    out += (o.side == Side::Buy ? "\"buy\"" : "\"sell\"");
    out += R"(,"type":)";
    switch (o.type) {
        case OrderType::Limit:  out += "\"limit\"";  break;
        case OrderType::Market: out += "\"market\""; break;
        case OrderType::Cancel: out += "\"cancel\""; break;
    }
    out += R"(,"price":)";
    out += std::to_string(o.price);
    out += R"(,"qty":)";
    out += std::to_string(o.quantity);
    out += "}";
}

/// Serializes an order to a minimal JSON string for WebSocket transmission.
/// Convenience overload; the hot path should prefer serialize_order_into().
inline std::string serialize_order(const Order& o) {
    std::string json;
    json.reserve(160);
    serialize_order_into(json, o);
    return json;
}

} // namespace bot_fleet::market
