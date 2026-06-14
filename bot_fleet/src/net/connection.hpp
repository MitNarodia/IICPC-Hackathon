#pragma once
#include <functional>

/// net/connection.hpp
/// ==================
/// WHY THIS FILE EXISTS:
///   Encapsulates all WebSocket protocol mechanics (handshake, send, receive,
///   close) behind a coroutine-friendly interface. Bots interact with this
///   class, never with raw sockets. This is the Connection Manager from the
///   architecture — owns transport, exposes async order submission.
///
/// CLASSES:
///   WsConnection — A single WebSocket session to the contestant's endpoint.
///     Wraps beast::websocket::stream, provides awaitable send/receive.
///
///   ConnectionPool — Manages a pool of WsConnections shared across bots.
///     Bots acquire a connection, send an order, await the response, release.
///     For the MVP: N connections shared across 1000 bots (not 1:1 — realistic
///     because exchanges limit connections per participant).
///
/// THREADING MODEL:
///   All operations execute on the io_context's thread via coroutines.
///   No background threads. The pool and connections are accessed sequentially
///   within the single-threaded event loop (or via strands if multi-threaded).
///
/// QUEUES:
///   ConnectionPool uses a simple available-connection queue (std::deque).
///   When all connections are busy, bot coroutines suspend until one is released.
///
/// ASYNC OPERATIONS:
///   - async_connect: TCP connect + WebSocket upgrade handshake
///   - async_send: Write order frame to WebSocket
///   - async_receive: Read response frame from WebSocket
///   - async_close: Graceful WebSocket close handshake

#include <boost/asio.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/websocket.hpp>
#include <boost/asio/awaitable.hpp>
#include <boost/asio/use_awaitable.hpp>
#include <deque>
#include <memory>
#include <string>
#include <string_view>

namespace net = boost::asio;
namespace beast = boost::beast;
namespace websocket = beast::websocket;
using tcp = net::ip::tcp;

namespace bot_fleet::net_layer {

/// A single WebSocket connection to the target endpoint.
class WsConnection {
public:
    explicit WsConnection(::net::io_context& ioc);

    /// Establish TCP connection and perform WebSocket handshake.
    ::net::awaitable<void> connect(std::string_view host, std::string_view port);

    /// Send a text message (serialized order JSON).
    ::net::awaitable<void> send(const std::string& message);

    /// Receive a text message (response JSON).
    ::net::awaitable<std::string> receive();

    /// Graceful close.
    ::net::awaitable<void> close();

    bool is_open() const;

private:
    websocket::stream<beast::tcp_stream> ws_;
    beast::flat_buffer read_buffer_;
};

/// Pool of WebSocket connections shared across many bots.
/// Bots check out a connection, use it, and return it.
class ConnectionPool {
public:
    ConnectionPool(::net::io_context& ioc, std::size_t pool_size);

    /// Connect all connections in the pool to the target.
    ::net::awaitable<void> connect_all(std::string_view host, std::string_view port);

    /// Acquire a connection. Suspends if none available.
    ::net::awaitable<WsConnection*> acquire();

    /// Release a connection back to the pool.
    void release(WsConnection* conn);

    /// Close all connections gracefully.
    ::net::awaitable<void> close_all();

    std::size_t pool_size() const { return connections_.size(); }

private:
    ::net::io_context& ioc_;
    std::vector<std::unique_ptr<WsConnection>> connections_;
    std::deque<WsConnection*> available_;

    // Waiters: coroutines suspended waiting for a connection.
    // Implemented via a simple channel/condition pattern.
    // std::deque<std::function<void(WsConnection*)>> waiters_;
    std::deque<std::move_only_function<void(WsConnection*)>> waiters_;
};

} // namespace bot_fleet::net_layer
