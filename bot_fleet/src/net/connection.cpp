/// net/connection.cpp
/// ==================
/// Implements WebSocket connection lifecycle and connection pooling.
///
/// KEY ASYNC OPERATIONS EXPLAINED:
///   1. connect(): Resolves hostname → TCP connect → WS handshake.
///      Each step is a co_await that suspends the coroutine and returns
///      control to io_context. When the kernel signals completion (via epoll),
///      io_context resumes the coroutine at the next line.
///
///   2. send(): Writes the serialized message as a WebSocket text frame.
///      Suspends until the kernel accepts the data into the TCP send buffer.
///
///   3. receive(): Suspends until a complete WebSocket frame arrives.
///      Beast handles frame reassembly internally.
///
///   4. ConnectionPool::acquire(): If a connection is available, returns
///      immediately. If not, the coroutine suspends and is queued in the
///      waiters_ deque. When another bot releases a connection, the first
///      waiter is resumed with that connection.

#include "net/connection.hpp"
#include <boost/asio/co_spawn.hpp>
#include <boost/asio/detached.hpp>
#include <stdexcept>

namespace bot_fleet::net_layer {

// ─────────────────────────────────────────────────────────────────────────────
// WsConnection
// ─────────────────────────────────────────────────────────────────────────────

WsConnection::WsConnection(::net::io_context& ioc)
    : ws_(::net::make_strand(ioc))
{}

::net::awaitable<void> WsConnection::connect(std::string_view host, std::string_view port) {
    auto executor = co_await ::net::this_coro::executor;
    tcp::resolver resolver(executor);

    // Step 1: DNS resolution (async, non-blocking)
    auto results = co_await resolver.async_resolve(host, port, ::net::use_awaitable);

    // Step 2: TCP connect to first available endpoint
    auto& tcp_layer = beast::get_lowest_layer(ws_);
    tcp_layer.expires_after(std::chrono::seconds(5));
    co_await tcp_layer.async_connect(results, ::net::use_awaitable);

    // Step 3: WebSocket upgrade handshake
    tcp_layer.expires_never();
    ws_.set_option(websocket::stream_base::timeout::suggested(beast::role_type::client));
    ws_.set_option(websocket::stream_base::decorator([](websocket::request_type& req) {
        req.set(boost::beast::http::field::user_agent, "BotFleet/0.1");
    }));

    co_await ws_.async_handshake(std::string(host), "/", ::net::use_awaitable);
}

::net::awaitable<void> WsConnection::send(const std::string& message) {
    ws_.text(true);  // Send as text frame (JSON)
    co_await ws_.async_write(::net::buffer(message), ::net::use_awaitable);
}

::net::awaitable<std::string> WsConnection::receive() {
    read_buffer_.clear();
    co_await ws_.async_read(read_buffer_, ::net::use_awaitable);
    co_return beast::buffers_to_string(read_buffer_.data());
}

::net::awaitable<void> WsConnection::close() {
    if (ws_.is_open()) {
        co_await ws_.async_close(websocket::close_code::normal, ::net::use_awaitable);
    }
}

bool WsConnection::is_open() const {
    return ws_.is_open();
}

// ─────────────────────────────────────────────────────────────────────────────
// ConnectionPool
// ─────────────────────────────────────────────────────────────────────────────

ConnectionPool::ConnectionPool(::net::io_context& ioc, std::size_t pool_size)
    : ioc_(ioc)
{
    connections_.reserve(pool_size);
    for (std::size_t i = 0; i < pool_size; ++i) {
        connections_.push_back(std::make_unique<WsConnection>(ioc));
    }
}

::net::awaitable<void> ConnectionPool::connect_all(std::string_view host, std::string_view port) {
    for (auto& conn : connections_) {
        co_await conn->connect(host, port);
        available_.push_back(conn.get());
    }
}

::net::awaitable<WsConnection*> ConnectionPool::acquire() {
    if (!available_.empty()) {
        auto* conn = available_.front();
        available_.pop_front();
        co_return conn;
    }

    // No connection available — suspend this coroutine until one is released.
    // We use a simple callback-based wait pattern compatible with the event loop.
    co_return co_await ::net::async_initiate<decltype(::net::use_awaitable), void(WsConnection*)>(
        [this](auto handler) {
            waiters_.push_back(
                [h = std::move(handler)](WsConnection* conn) mutable {
                    std::move(h)(conn);
                }
            );
        },
        ::net::use_awaitable
    );
}

void ConnectionPool::release(WsConnection* conn) {
    if (!waiters_.empty()) {
        // Wake up the first waiter directly — zero-copy handoff.
        auto waiter = std::move(waiters_.front());
        waiters_.pop_front();
        waiter(conn);
    } else {
        available_.push_back(conn);
    }
}

::net::awaitable<void> ConnectionPool::close_all() {
    for (auto& conn : connections_) {
        if (conn->is_open()) {
            try {
                co_await conn->close();
            } catch (...) {
                // Best-effort close; ignore errors during shutdown
            }
        }
    }
}

} // namespace bot_fleet::net_layer
