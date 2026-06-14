#pragma once

/// tests/support/market_invariants.hpp
/// ===================================
/// Shared correctness predicate for the MarketSimulator's published state.
///
/// The simulator couples three fields atomically on every regime change:
///     calm regime      -> volatility == 0.01 && spread == 0.02
///     volatile regime  -> volatility == 0.05 && spread == 0.10
/// and floors mid_price at 1.0 (and it must stay finite under GBM).
///
/// A *correctly* published SeqLock state ALWAYS satisfies this predicate.
/// A torn read — fields drawn from two different writer updates, or a
/// half-written 8-byte double — will violate it. That is exactly why this
/// predicate is the tear detector used by the concurrency tests.
///
/// NOTE on float equality: 0.01/0.02/0.05/0.10 are stored as exact literals by
/// the writer (never computed), so `==` is the correct, intended comparison.

#include "market/market_simulator.hpp"
#include <cmath>

namespace bot_fleet::test_support {

inline bool is_consistent(const market::MarketState& s) {
    if (!std::isfinite(s.mid_price) || s.mid_price < 1.0) {
        return false;
    }
    if (s.is_volatile) {
        return s.volatility == 0.05 && s.spread == 0.10;
    }
    return s.volatility == 0.01 && s.spread == 0.02;
}

} // namespace bot_fleet::test_support
