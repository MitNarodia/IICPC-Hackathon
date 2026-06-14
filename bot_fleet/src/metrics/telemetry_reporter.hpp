#pragma once

/// metrics/telemetry_reporter.hpp
/// ==============================
/// CROSS-TRACK INTEGRATION: Track 2 → Track 3
///
/// This reporter posts the per-window AggregateView to Track 3's ingestion
/// service in the exact JSON shape that `/v1/track2/bot-metrics` expects.
/// It runs asynchronously on its own strand so it never blocks the hot path.
///
/// THREADING: The reporter is called from the reporter thread (cold path).
/// It holds its own Beast HTTP client and serializes JSON on-the-spot.
///
/// CONFIGURATION:
///   TRACK3_INGEST_URL  — e.g. "http://localhost:8081" (ingestion-service)
///   TRACK3_RUN_ID      — benchmark run identifier
///   TRACK3_SUBMISSION_ID — the contestant's submission under test

#include "metrics/metrics_aggregator.hpp"
#include <boost/asio.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/http.hpp>
#include <boost/beast/version.hpp>
#include <string>
#include <cstdint>
#include <atomic>

namespace net = boost::asio;
namespace beast = boost::beast;
namespace http = beast::http;
using tcp = net::ip::tcp;

namespace bot_fleet::metrics {

/// Configuration for the Track 3 telemetry reporter.
struct TelemetryReporterConfig {
    std::string ingest_host = "localhost";
    std::string ingest_port = "8081";
    std::string run_id = "run-default";
    std::string submission_id = "sub-default";
    std::string source = "bot-fleet";
    bool enabled = false;  // only reports if TRACK3_INGEST_URL is set
};

/// Sends AggregateView snapshots to Track 3's ingestion service.
/// Fire-and-forget: if the ingest is unreachable, metrics are silently dropped.
class TelemetryReporter {
public:
    explicit TelemetryReporter(TelemetryReporterConfig cfg);

    /// Post one aggregated window to Track 3. Thread-safe, non-blocking on the
    /// hot path (serializes JSON and posts over a short-lived TCP connection).
    void report(const AggregateView& view, uint32_t shard_id = 0);

    /// How many reports were successfully sent.
    uint64_t reports_sent() const { return sent_.load(std::memory_order_relaxed); }
    uint64_t reports_failed() const { return failed_.load(std::memory_order_relaxed); }

private:
    TelemetryReporterConfig cfg_;
    std::atomic<uint64_t> seq_{0};
    std::atomic<uint64_t> sent_{0};
    std::atomic<uint64_t> failed_{0};
};

/// Parse TRACK3_INGEST_URL, TRACK3_RUN_ID, TRACK3_SUBMISSION_ID from environment.
TelemetryReporterConfig load_reporter_config_from_env();

} // namespace bot_fleet::metrics
