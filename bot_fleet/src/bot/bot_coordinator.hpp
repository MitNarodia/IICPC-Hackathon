#pragma once

/// bot/bot_coordinator.hpp
/// =======================
/// WHY THIS FILE EXISTS:
///   Orchestrates the lifecycle of the entire bot fleet using a
///   THREAD-PER-CORE (share-nothing) model.
///
///   This is the "Bot Coordinator" from the architecture. In the distributed
///   version, this would be a separate gRPC service. For the MVP, it's an
///   in-process orchestrator that proves the architecture works — now scaled
///   across all cores of one host.
///
/// CLASSES:
///   RunConfig      — Parameters for a single stress-test run.
///   Worker         — One pinned core: its own io_context, pool, metrics, bots.
///   BotCoordinator — Owns the workers, the shared market sim, the aggregator.
///
/// THREADING MODEL (NEW):
///   - N worker threads, each pinned to a core, each running its OWN
///     io_context. A worker owns its ConnectionPool, its MetricsCollector,
///     and a strided subset of bots. Nothing on the hot path is shared
///     between workers => no locks, no cross-core cache-line bouncing.
///   - The MarketSimulator is SHARED and read by all workers via a SeqLock.
///     Exactly ONE worker (index 0) is the market writer (single-writer
///     invariant of the SeqLock).
///   - One dedicated reporter thread periodically flushes the aggregator.
///   Total threads: N workers + 1 reporter + the main thread (which blocks
///   on joins).
///
/// QUEUES:
///   Each worker delegates to its ConnectionPool's internal queue. The
///   aggregator merges snapshots without a queue.
///
/// ASYNC OPERATIONS:
///   - Per worker: pool connect, bot coroutines, a market-tick loop (worker 0
///     only), and a metrics-snapshot loop.
///   - Reporter thread: a flush timer loop.

#include "bot/bot.hpp"
#include "market/market_simulator.hpp"
#include "metrics/metrics_collector.hpp"
#include "metrics/metrics_aggregator.hpp"
#include "metrics/telemetry_reporter.hpp"
#include "net/connection.hpp"

#include <boost/asio/io_context.hpp>
#include <boost/asio/steady_timer.hpp>
#include <cstddef>
#include <string>
#include <thread>
#include <vector>
#include <memory>

// Forward-declared test shim, befriended below so the persona generator and
// the worker-count capping logic can be unit-tested without public getters.
namespace bot_fleet::test_access { class CoordinatorProbe; }

namespace bot_fleet::bot {

/// Configuration for a complete stress-test run.
struct RunConfig {
    std::string target_host = "127.0.0.1";
    std::string target_port = "9090";

    uint32_t num_bots = 1000;
    uint32_t connection_pool_size = 50;  // TOTAL connections, split across workers
    uint64_t orders_per_bot = 100;       // Total orders before stopping

    // 0 => use std::thread::hardware_concurrency(). Capped to num_bots so no
    // worker ends up with an empty shard (which would otherwise never finish).
    unsigned num_workers = 0;

    // Market simulator tick interval
    std::chrono::milliseconds market_tick_interval{100};

    // Metrics reporting interval
    std::chrono::seconds metrics_report_interval{5};

    // Deterministic seed for reproducibility
    uint64_t run_seed = 12345;
};

/// One pinned core's worth of load generation. Share-nothing on the hot path.
struct Worker {
    explicit Worker(std::size_t conns)
        : pool(std::make_unique<net_layer::ConnectionPool>(ioc, conns))
        , metrics(std::make_unique<metrics::MetricsCollector>())
        , market_timer(ioc)
        , metrics_timer(ioc)
    {}

    ::net::io_context ioc;                                 // per-worker event loop
    std::unique_ptr<net_layer::ConnectionPool> pool;      // per-worker connections
    std::unique_ptr<metrics::MetricsCollector> metrics;   // per-worker telemetry
    std::vector<std::unique_ptr<Bot>> bots;               // this shard's bots

    ::net::steady_timer market_timer;                     // used by writer only
    ::net::steady_timer metrics_timer;                    // snapshot cadence

    std::size_t bots_remaining = 0;   // touched only on this worker's thread
    bool is_market_writer = false;    // exactly one worker sets this true
    unsigned index = 0;
};

/// Orchestrates the entire bot fleet lifecycle across cores.
class BotCoordinator {
    // Test-only access to private config generation + worker-count capping.
    friend class ::bot_fleet::test_access::CoordinatorProbe;

public:
    explicit BotCoordinator(RunConfig config);

    /// Execute the full run: spawn workers, generate load, join, report.
    void execute();

private:
    /// Create diverse bot configs based on persona distribution (seeded).
    std::vector<BotConfig> generate_bot_configs();

    /// Worker thread body: builds the shard, drives its io_context to drain.
    void run_worker(Worker& w, const std::vector<BotConfig>& all_configs);

    /// Coroutine: ticks the SHARED market simulator (worker 0 only).
    ::net::awaitable<void> market_tick_loop(Worker& w);

    /// Coroutine: snapshots this shard's metrics and submits to the aggregator.
    ::net::awaitable<void> metrics_snapshot_loop(Worker& w);

    RunConfig config_;
    unsigned num_workers_ = 1;

    // Shared, long-lived: created before workers, destroyed after all joins.
    std::unique_ptr<market::MarketSimulator> market_;
    std::unique_ptr<metrics::MetricsAggregator> aggregator_;
    std::unique_ptr<metrics::TelemetryReporter> reporter_;

    std::vector<std::unique_ptr<Worker>> workers_;
};

} // namespace bot_fleet::bot
