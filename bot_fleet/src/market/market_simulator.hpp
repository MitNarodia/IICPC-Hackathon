#pragma once

/// market/market_simulator.hpp
/// ===========================
/// WHY THIS FILE EXISTS:
///   Generates realistic synthetic market conditions that drive bot behavior.
///   Without this, bots would send uniform random orders — trivial to handle
///   and unrealistic. The simulator produces price trajectories and volatility
///   regimes that create correlated bursts and quiet periods.
///
/// CLASSES:
///   MarketSimulator — Single-writer market state producer.
///     Updated by one thread/coroutine; read (acquire) by all bot coroutines.
///     No mutex needed — atomic publish with release semantics.
///
/// THREADING MODEL:
///   One dedicated coroutine updates market state every tick interval.
///   Bot coroutines read the latest snapshot via atomic load (acquire).

#include <atomic>
#include <cstdint>
#include <random>
#include <cmath>

namespace bot_fleet::market {

/// Immutable snapshot of market conditions at a point in time.
/// Small enough to be published atomically (on x86, aligned 64-bit stores are atomic).
struct MarketState {
    double mid_price    = 100.0;   // Current mid price
    double spread       = 0.02;    // Bid-ask spread
    double volatility   = 0.01;    // Current vol regime (σ per tick)
    bool   is_volatile  = false;   // High-vol regime flag
};

/// Generates evolving market microstructure.
/// Uses geometric Brownian motion with regime switching.
class MarketSimulator {
public:
    explicit MarketSimulator(uint64_t seed = 42)
        : rng_(seed)
        , normal_(0.0, 1.0)
        , uniform_(0.0, 1.0) {}

    /// Advance market state by one tick. Called periodically by the simulator coroutine.
    ///
    /// SEQLOCK WRITER PROTOCOL (single writer):
    ///   1. Bump seq to odd  → signals "write in progress".
    ///   2. Mutate the plain payload between two release fences.
    ///   3. Bump seq to even → signals "write complete".
    /// Readers never block the writer and the writer never blocks readers.
    void tick() {
        // Begin write: make sequence odd.
        const uint64_t s = seq_.load(std::memory_order_relaxed);
        seq_.store(s + 1, std::memory_order_relaxed);
        std::atomic_thread_fence(std::memory_order_release);

        // The writer is the sole mutator, so reading state_ here is race-free.
        MarketState state = state_;

        // Regime switching: 1% chance per tick to flip volatility regime
        if (uniform_(rng_) < 0.01) {
            state.is_volatile = !state.is_volatile;
            state.volatility = state.is_volatile ? 0.05 : 0.01;
            state.spread = state.is_volatile ? 0.10 : 0.02;
        }

        // GBM price evolution: dS = σ * S * dW
        double dw = normal_(rng_);
        state.mid_price *= (1.0 + state.volatility * dw);

        // Floor price at 1.0 to avoid nonsense
        state.mid_price = std::max(state.mid_price, 1.0);

        state_ = state;

        // End write: make sequence even again.
        std::atomic_thread_fence(std::memory_order_release);
        seq_.store(s + 2, std::memory_order_relaxed);
        tick_count_.fetch_add(1, std::memory_order_relaxed);
    }

    /// Read current market snapshot.
    ///
    /// SEQLOCK READER PROTOCOL: retry while the writer is mid-update.
    /// Truly wait-free for readers in the common (uncontended) case: a single
    /// acquire load, a payload copy, and a second acquire load with no atomic
    /// RMW and no shared cache line being written by readers. This is the
    /// fix for the previous std::atomic<MarketState> design, which silently
    /// took an internal lock for >16-byte payloads and serialized every bot.
    MarketState snapshot() const {
        MarketState out;
        uint64_t before, after;
        do {
            before = seq_.load(std::memory_order_acquire);
            if (before & 1ULL) continue;                 // writer in progress
            out = state_;                                // plain payload copy
            std::atomic_thread_fence(std::memory_order_acquire);
            after = seq_.load(std::memory_order_acquire);
        } while ((before & 1ULL) || before != after);    // torn read → retry
        return out;
    }

    uint64_t tick_count() const { return tick_count_.load(std::memory_order_relaxed); }

private:
    // SeqLock: a single 64-bit sequence guards a plain payload. Readers spin
    // only during the writer's brief window. Aligned to its own cache line so
    // the writer's seq bumps never false-share with neighbouring data.
    alignas(64) std::atomic<uint64_t> seq_{0};
    MarketState state_{};                 // plain payload, guarded by seq_
    std::atomic<uint64_t> tick_count_{0};

    std::mt19937_64 rng_;
    std::normal_distribution<double> normal_;
    std::uniform_real_distribution<double> uniform_;
};

} // namespace bot_fleet::market
