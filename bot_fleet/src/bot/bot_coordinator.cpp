/// bot/bot_coordinator.cpp
/// =======================
/// Implements the THREAD-PER-CORE orchestration logic for the bot fleet.
///
/// EXECUTION TIMELINE:
///   1. Construct shared state (market sim, aggregator) and N Workers.
///   2. Launch a reporter thread that flushes the aggregator periodically.
///   3. Launch N worker threads. Each pins to a core and runs its own
///      io_context, which: connects its pool, spawns its strided bot shard,
///      a metrics-snapshot loop, and (worker 0 only) the market-tick loop.
///   4. A worker's io_context drains and returns once all its bots finish
///      (their completion handlers cancel the support timers).
///   5. Join workers, stop the reporter, take a final per-shard snapshot,
///      and print the authoritative cumulative report.
///
/// WHY THREAD-PER-CORE:
///   A single io_context is bounded by one core's syscall + serialization
///   throughput. Giving each core its own io_context, pool, and metrics
///   removes that ceiling and keeps the hot path share-nothing (no locks,
///   no cross-core cache-line bouncing). The only shared object on the hot
///   path is the read-only market state, which is a SeqLock.

#include "bot/bot_coordinator.hpp"
#include <boost/asio/co_spawn.hpp>
#include <boost/asio/detached.hpp>
#include <boost/asio/steady_timer.hpp>
#include <boost/asio/use_awaitable.hpp>
#include <algorithm>
#include <atomic>
#include <chrono>
#include <iostream>
#include <thread>

#if defined(__linux__)
#include <pthread.h>
#include <sched.h>
#endif

namespace bot_fleet::bot {
namespace {

/// Pin the calling thread to a specific core. On Linux this reduces scheduler
/// migration and tightens tail latency. Off Linux it is a no-op (the dev
/// machine is Windows; the deployment target is Linux/K8s).
void pin_current_thread_to_core(unsigned core) {
#if defined(__linux__)
    cpu_set_t set;
    CPU_ZERO(&set);
    CPU_SET(core % CPU_SETSIZE, &set);
    pthread_setaffinity_np(pthread_self(), sizeof(set), &set);
#else
    (void)core;
#endif
}

} // namespace

BotCoordinator::BotCoordinator(RunConfig config)
    : config_(std::move(config))
{
    unsigned hw = std::thread::hardware_concurrency();
    if (hw == 0) hw = 1;
    num_workers_ = config_.num_workers ? config_.num_workers : hw;

    // Cap workers to bot count so no worker gets an empty shard (an empty
    // shard would have nothing to cancel its support timers and would hang).
    if (config_.num_bots > 0) {
        num_workers_ = std::min<unsigned>(num_workers_, config_.num_bots);
    }
    if (num_workers_ == 0) num_workers_ = 1;

    market_ = std::make_unique<market::MarketSimulator>(config_.run_seed);
    aggregator_ = std::make_unique<metrics::MetricsAggregator>(num_workers_);
    reporter_ = std::make_unique<metrics::TelemetryReporter>(
        metrics::load_reporter_config_from_env());

    const std::size_t conns_per_worker =
        std::max<std::size_t>(1, config_.connection_pool_size / num_workers_);

    workers_.reserve(num_workers_);
    for (unsigned i = 0; i < num_workers_; ++i) {
        auto w = std::make_unique<Worker>(conns_per_worker);
        w->index = i;
        w->is_market_writer = (i == 0);  // single-writer invariant of the SeqLock
        workers_.push_back(std::move(w));
    }
}

void BotCoordinator::execute() {
    std::cout << "[Coordinator] Starting bot fleet run (thread-per-core)\n";
    std::cout << "[Coordinator] Target: " << config_.target_host << ":" << config_.target_port << "\n";
    std::cout << "[Coordinator] Workers (cores): " << num_workers_ << "\n";
    std::cout << "[Coordinator] Bots: " << config_.num_bots << "\n";
    std::cout << "[Coordinator] Connections (total): " << config_.connection_pool_size << "\n";
    std::cout << "[Coordinator] Orders/bot: " << config_.orders_per_bot << "\n";
    std::cout << "[Coordinator] Total orders planned: "
              << static_cast<uint64_t>(config_.num_bots) * config_.orders_per_bot << "\n\n";

    // Bot configs are generated ONCE, centrally and seeded, so each bot_id maps
    // to the same persona regardless of how many workers we shard across.
    auto all_configs = generate_bot_configs();

    // ── Reporter thread: periodically flush the merged live window ──────────
    std::atomic<bool> reporter_stop{false};
    std::thread reporter([this, &reporter_stop] {
        using namespace std::chrono;
        auto next = steady_clock::now() + config_.metrics_report_interval;
        while (!reporter_stop.load(std::memory_order_acquire)) {
            std::this_thread::sleep_for(milliseconds(100));
            if (steady_clock::now() >= next) {
                // Capture the window view BEFORE flush resets it.
                auto view = aggregator_->window();
                aggregator_->flush();
                // Cross-track: report to Track 3 ingestion (no-op if disabled).
                reporter_->report(view);
                next += config_.metrics_report_interval;
            }
        }
    });

    // ── Launch one pinned worker thread per core ───────────────────────────
    std::vector<std::thread> threads;
    threads.reserve(num_workers_);
    for (unsigned i = 0; i < num_workers_; ++i) {
        threads.emplace_back(&BotCoordinator::run_worker, this,
                             std::ref(*workers_[i]), std::cref(all_configs));
    }

    // ── Wait for all load generation to complete ───────────────────────────
    for (auto& t : threads) t.join();

    // ── Stop the reporter (workers are done, no more submits will arrive) ──
    reporter_stop.store(true, std::memory_order_release);
    reporter.join();

    // ── Drain the final partial window from each shard (race-free now) ─────
    for (auto& w : workers_) {
        aggregator_->submit(w->metrics->snapshot_and_reset());
    }

    std::cout << "\n[Coordinator] All bots completed.\n";
    aggregator_->final_report();
    std::cout << "[Coordinator] Run finished.\n";
}

void BotCoordinator::run_worker(Worker& w, const std::vector<BotConfig>& all_configs) {
    // Build the shard on the worker's own io_context, then drive it to drain.
    ::net::co_spawn(w.ioc, [this, &w, &all_configs]() -> ::net::awaitable<void> {
        // Phase 1: connect this worker's pool.
        try {
            co_await w.pool->connect_all(config_.target_host, config_.target_port);
        } catch (const std::exception& e) {
            std::cerr << "[worker " << w.index << "] connect failed: " << e.what() << "\n";
            w.metrics_timer.cancel();
            if (w.is_market_writer) w.market_timer.cancel();
            co_return;
        }

        // Phase 2: support coroutines. Only worker 0 writes the shared market.
        if (w.is_market_writer) {
            ::net::co_spawn(w.ioc, market_tick_loop(w), ::net::detached);
        }
        ::net::co_spawn(w.ioc, metrics_snapshot_loop(w), ::net::detached);

        // Phase 3: spawn this shard's strided subset of bots. Each completion
        // decrements the outstanding count; the last one cancels the support
        // timers so the io_context can drain and run() can return.
        for (uint32_t id = w.index; id < config_.num_bots; id += num_workers_) {
            ++w.bots_remaining;
            auto bot = std::make_unique<Bot>(all_configs[id], *w.pool, *market_, *w.metrics);
            auto* bot_ptr = bot.get();
            w.bots.push_back(std::move(bot));

            ::net::co_spawn(
                w.ioc,
                bot_ptr->run(config_.orders_per_bot),
                [this, &w](std::exception_ptr) {
                    if (--w.bots_remaining == 0) {
                        w.metrics_timer.cancel();
                        if (w.is_market_writer) w.market_timer.cancel();
                    }
                });
        }

        // Defensive: a shard with zero bots must still let its io_context drain.
        if (w.bots_remaining == 0) {
            w.metrics_timer.cancel();
            if (w.is_market_writer) w.market_timer.cancel();
        }
    }, ::net::detached);

    pin_current_thread_to_core(w.index);
    w.ioc.run();  // returns once the shard's work is fully drained
}

std::vector<BotConfig> BotCoordinator::generate_bot_configs() {
    std::vector<BotConfig> configs;
    configs.reserve(config_.num_bots);

    std::mt19937_64 rng(config_.run_seed);
    std::uniform_real_distribution<double> dist(0.0, 1.0);

    for (uint32_t i = 0; i < config_.num_bots; ++i) {
        BotConfig cfg;
        cfg.bot_id = i;

        // Persona assignment based on realistic market participant mix:
        // 10% market makers (fast, high limit ratio)
        // 20% momentum traders (medium speed, high market ratio)
        // 30% noise traders (slow, random)
        // 40% passive limit providers (slow, high limit ratio)
        double persona_roll = dist(rng);

        if (persona_roll < 0.10) {
            // Market maker: fast, mostly limits, narrow spread
            cfg.mean_interval_ms = 2.0 + dist(rng) * 3.0;  // 2-5ms
            cfg.limit_ratio = 0.8;
            cfg.market_ratio = 0.05;
            cfg.cancel_ratio = 0.15;
            cfg.price_aggressiveness = 0.5;
        } else if (persona_roll < 0.30) {
            // Momentum taker: medium speed, market orders
            cfg.mean_interval_ms = 10.0 + dist(rng) * 20.0; // 10-30ms
            cfg.limit_ratio = 0.2;
            cfg.market_ratio = 0.7;
            cfg.cancel_ratio = 0.1;
            cfg.price_aggressiveness = 2.0;
        } else if (persona_roll < 0.60) {
            // Noise trader: slow, random behavior
            cfg.mean_interval_ms = 50.0 + dist(rng) * 200.0; // 50-250ms
            cfg.limit_ratio = 0.4;
            cfg.market_ratio = 0.4;
            cfg.cancel_ratio = 0.2;
            cfg.price_aggressiveness = 1.5;
        } else {
            // Passive provider: slow, mostly limit orders at wide spread
            cfg.mean_interval_ms = 100.0 + dist(rng) * 400.0; // 100-500ms
            cfg.limit_ratio = 0.85;
            cfg.market_ratio = 0.05;
            cfg.cancel_ratio = 0.10;
            cfg.price_aggressiveness = 3.0;
        }

        cfg.min_qty = 1;
        cfg.max_qty = static_cast<uint32_t>(10 + dist(rng) * 90); // 10-100

        configs.push_back(cfg);
    }

    return configs;
}

::net::awaitable<void> BotCoordinator::market_tick_loop(Worker& w) {
    // SINGLE-WRITER: only the market-writer worker runs this loop, satisfying
    // the SeqLock's single-writer invariant for the shared MarketSimulator.
    while (true) {
        w.market_timer.expires_after(config_.market_tick_interval);
        try {
            co_await w.market_timer.async_wait(::net::use_awaitable);
        } catch (const boost::system::system_error&) {
            break;  // cancelled during shutdown
        }
        market_->tick();
    }
}

::net::awaitable<void> BotCoordinator::metrics_snapshot_loop(Worker& w) {
    // Runs on the worker's OWN thread: snapshot_and_reset() copies this shard's
    // histogram + counters race-free, then hands them to the aggregator on a
    // cold path (the only lock in the system, never touched by the hot path).
    while (true) {
        w.metrics_timer.expires_after(config_.metrics_report_interval);
        try {
            co_await w.metrics_timer.async_wait(::net::use_awaitable);
        } catch (const boost::system::system_error&) {
            break;  // cancelled during shutdown
        }
        aggregator_->submit(w.metrics->snapshot_and_reset());
    }
}

} // namespace bot_fleet::bot
