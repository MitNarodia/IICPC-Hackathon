/// mock_server/mock_exchange.cpp
/// =============================
/// WHY THIS FILE EXISTS:
///   You cannot test the bot fleet without a target to hit. This mock exchange
///   accepts WebSocket connections, receives order JSON, and responds with
///   acknowledgments after a configurable simulated processing delay.
///   It validates the architecture works end-to-end without needing a real
///   contestant submission.
///
/// CLASSES:
///   Session — One WebSocket session (accepts, reads orders, writes acks).
///   MockExchange — Listens on a port and accepts connections.
///
/// THREADING MODEL:
///   Single io_context, single thread — same pattern as the bot fleet.
///   Each accepted connection spawns a coroutine (not a thread).
///   Can handle thousands of concurrent connections.
///
/// QUEUES: None. Direct request-response within each session coroutine.
///
/// ASYNC OPERATIONS:
///   - async_accept: Waits for TCP connection
///   - async_accept (WS): Performs WebSocket upgrade
///   - async_read: Reads order frame
///   - async_write: Sends response frame
///
/// SIMULATED BEHAVIOR:
///   - Adds a random 50-500μs delay to simulate matching engine processing
///   - Randomly accepts (90%), rejects (5%), or fills (5%) orders
///   - This produces realistic latency distributions for testing

#include <boost/asio.hpp>
#include <boost/asio/awaitable.hpp>
#include <boost/asio/co_spawn.hpp>
#include <boost/asio/detached.hpp>
#include <boost/asio/use_awaitable.hpp>
#include <boost/asio/steady_timer.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/websocket.hpp>
#include <iostream>
#include <random>
#include <string>
#include <atomic>
#include <chrono>

namespace net = boost::asio;
namespace beast = boost::beast;
namespace websocket = beast::websocket;
using tcp = net::ip::tcp;

// Global counters for server-side stats
static std::atomic<uint64_t> g_total_requests{0};
static std::atomic<uint64_t> g_active_connections{0};

/// Handle a single WebSocket session.
net::awaitable<void> handle_session(websocket::stream<beast::tcp_stream> ws) {
    g_active_connections.fetch_add(1, std::memory_order_relaxed);

    try {
        // Accept the WebSocket upgrade
        co_await ws.async_accept(net::use_awaitable);

        // Per-session RNG for simulated delays and outcomes
        std::mt19937_64 rng(std::chrono::steady_clock::now().time_since_epoch().count());
        std::uniform_int_distribution<int> delay_dist(50, 500);  // μs
        std::uniform_real_distribution<double> outcome_dist(0.0, 1.0);

        beast::flat_buffer buffer;

        while (true) {
            // Read one order frame
            buffer.clear();
            co_await ws.async_read(buffer, net::use_awaitable);
            g_total_requests.fetch_add(1, std::memory_order_relaxed);

            // Simulate matching engine processing delay (50-500μs)
            auto delay_us = delay_dist(rng);
            net::steady_timer timer(co_await net::this_coro::executor);
            timer.expires_after(std::chrono::microseconds(delay_us));
            co_await timer.async_wait(net::use_awaitable);

            // Determine outcome
            double roll = outcome_dist(rng);
            std::string status;
            if (roll < 0.90) {
                status = "accepted";
            } else if (roll < 0.95) {
                status = "rejected";
            } else {
                status = "filled";
            }

            // Construct minimal response JSON
            std::string response = R"({"status":")" + status + R"(","ts":)" +
                std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()) +
                "}";

            // Send response
            ws.text(true);
            co_await ws.async_write(net::buffer(response), net::use_awaitable);
        }
    } catch (const beast::system_error& se) {
        if (se.code() != websocket::error::closed &&
            se.code() != net::error::eof &&
            se.code() != net::error::connection_reset) {
            std::cerr << "[Session] Error: " << se.what() << "\n";
        }
    } catch (const std::exception& e) {
        std::cerr << "[Session] Exception: " << e.what() << "\n";
    }

    g_active_connections.fetch_sub(1, std::memory_order_relaxed);
}

/// Accept loop: listens for TCP connections and spawns session coroutines.
net::awaitable<void> accept_loop(tcp::acceptor& acceptor) {
    while (true) {
        auto socket = co_await acceptor.async_accept(net::use_awaitable);
        auto ws = websocket::stream<beast::tcp_stream>(std::move(socket));

        net::co_spawn(
            co_await net::this_coro::executor,
            handle_session(std::move(ws)),
            net::detached
        );
    }
}

/// Periodic stats printer.
net::awaitable<void> stats_loop() {
    net::steady_timer timer(co_await net::this_coro::executor);
    while (true) {
        timer.expires_after(std::chrono::seconds(5));
        co_await timer.async_wait(net::use_awaitable);

        std::cout << "[MockExchange] Active connections: "
                  << g_active_connections.load(std::memory_order_relaxed)
                  << " | Total requests: "
                  << g_total_requests.load(std::memory_order_relaxed) << "\n";
    }
}

int main(int argc, char* argv[]) {
    uint16_t port = 9090;
    if (argc > 1) {
        port = static_cast<uint16_t>(std::stoi(argv[1]));
    }

    std::cout << "[MockExchange] Starting on port " << port << "\n";
    std::cout << "[MockExchange] Simulating 50-500μs processing delay\n";
    std::cout << "[MockExchange] Response mix: 90% accepted, 5% rejected, 5% filled\n\n";

    try {
        net::io_context ioc;

        tcp::acceptor acceptor(ioc, tcp::endpoint(tcp::v4(), port));
        acceptor.set_option(net::socket_base::reuse_address(true));

        net::co_spawn(ioc, accept_loop(acceptor), net::detached);
        net::co_spawn(ioc, stats_loop(), net::detached);

        ioc.run();
    } catch (const std::exception& e) {
        std::cerr << "[FATAL] " << e.what() << "\n";
        return 1;
    }

    return 0;
}
