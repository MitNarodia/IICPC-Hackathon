/// tests/concurrency/test_clean_shutdown.cpp
/// =========================================
/// Proves the coordinator's event loops drain and the process terminates.
///
/// WHY THIS MATTERS:
///   The original design used infinite `while(true)` timer loops that would
///   keep each io_context alive forever, so the run never ended. The redesign
///   fixes this: each bot's completion handler decrements an outstanding count
///   and the last one cancels the shard's support timers, letting io_context.run()
///   return so the worker thread can join. A regression here means a hung load
///   test that never reports results.
///
///   We exercise the REAL shutdown path without needing a live server: pointed
///   at a closed port, connect_all() fails fast (connection refused), the setup
///   coroutine cancels the timers, the io_context drains, every worker thread
///   joins, the reporter thread stops, and execute() returns. If that whole
///   sequence does not complete within a generous timeout, the test fails
///   instead of hanging the suite.

#include <gtest/gtest.h>

#include "bot/bot_coordinator.hpp"

#include <atomic>
#include <chrono>
#include <iostream>
#include <memory>
#include <sstream>
#include <thread>

using bot_fleet::bot::BotCoordinator;
using bot_fleet::bot::RunConfig;

TEST(CleanShutdown, TerminatesWhenTargetUnreachable) {
    RunConfig cfg;
    cfg.target_host = "127.0.0.1";
    cfg.target_port = "65501";        // nothing listening -> fast connection refused
    cfg.num_bots = 4;
    cfg.num_workers = 2;
    cfg.connection_pool_size = 2;
    cfg.orders_per_bot = 5;

    // Shared with the runner thread; a shared_ptr keeps it alive even on the
    // (failing) hang path where we must detach rather than join.
    auto finished = std::make_shared<std::atomic<bool>>(false);

    // Silence the coordinator's banner/report during the test.
    std::ostringstream sink;
    auto* old_buf = std::cout.rdbuf(sink.rdbuf());

    std::thread runner([cfg, finished]() {
        BotCoordinator coordinator(cfg);
        coordinator.execute();
        finished->store(true, std::memory_order_release);
    });

    using namespace std::chrono;
    const auto deadline = steady_clock::now() + seconds(30);
    while (!finished->load(std::memory_order_acquire) &&
           steady_clock::now() < deadline) {
        std::this_thread::sleep_for(milliseconds(20));
    }

    const bool ok = finished->load(std::memory_order_acquire);
    std::cout.rdbuf(old_buf);  // restore before asserting/printing

    if (ok) {
        runner.join();
    } else {
        // Hung: detach so the std::thread destructor doesn't std::terminate.
        // The captured cfg copy + shared_ptr keep the detached thread safe.
        runner.detach();
    }
    ASSERT_TRUE(ok) << "execute() did not return within 30s (shutdown hang)";
}
