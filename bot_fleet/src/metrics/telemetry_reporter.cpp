/// metrics/telemetry_reporter.cpp
/// ==============================
/// CROSS-TRACK INTEGRATION: Track 2 → Track 3
///
/// Posts the bot fleet's AggregateView to Track 3's ingestion service using a
/// synchronous Beast HTTP POST. This runs on the reporter thread (cold path,
/// once every 5s), so blocking for ~1ms is acceptable.

#include "metrics/telemetry_reporter.hpp"
#include <boost/asio/connect.hpp>
#include <chrono>
#include <cstdlib>
#include <iostream>
#include <sstream>
#include <string>

namespace bot_fleet::metrics {

TelemetryReporter::TelemetryReporter(TelemetryReporterConfig cfg)
    : cfg_(std::move(cfg)) {}

void TelemetryReporter::report(const AggregateView& view, uint32_t shard_id) {
    if (!cfg_.enabled) return;

    uint64_t s = seq_.fetch_add(1, std::memory_order_relaxed);

    // Serialize the AggregateView into the JSON shape that Track 3's
    // /v1/track2/bot-metrics expects (track2BotMetricsIn).
    auto now_ns = std::chrono::duration_cast<std::chrono::nanoseconds>(
        std::chrono::system_clock::now().time_since_epoch()).count();
    auto start_ns = now_ns - static_cast<int64_t>(view.seconds * 1e9);

    std::ostringstream json;
    json << "{"
         << "\"run_id\":\"" << cfg_.run_id << "\","
         << "\"submission_id\":\"" << cfg_.submission_id << "\","
         << "\"source\":\"" << cfg_.source << "\","
         << "\"seq\":" << s << ","
         << "\"shard_id\":" << shard_id << ","
         << "\"transactions\":" << view.txns << ","
         << "\"errors\":" << view.errors << ","
         << "\"timeouts\":" << view.timeouts << ","
         << "\"seconds\":" << view.seconds << ","
         << "\"p50_us\":" << view.p50 << ","
         << "\"p90_us\":" << view.p90 << ","
         << "\"p99_us\":" << view.p99 << ","
         << "\"mean_us\":" << view.mean << ","
         << "\"window_start_ts\":" << start_ns << ","
         << "\"window_end_ts\":" << now_ns
         << "}";
    std::string body = json.str();

    try {
        net::io_context ioc;
        tcp::resolver resolver(ioc);
        beast::tcp_stream stream(ioc);

        auto results = resolver.resolve(cfg_.ingest_host, cfg_.ingest_port);
        stream.connect(results);
        stream.expires_after(std::chrono::seconds(3));

        http::request<http::string_body> req(http::verb::post, "/v1/track2/bot-metrics", 11);
        req.set(http::field::host, cfg_.ingest_host);
        req.set(http::field::content_type, "application/json");
        req.set(http::field::user_agent, "BotFleet/0.1");
        req.body() = body;
        req.prepare_payload();

        http::write(stream, req);

        beast::flat_buffer buffer;
        http::response<http::string_body> res;
        http::read(stream, buffer, res);

        if (res.result_int() >= 200 && res.result_int() < 300) {
            sent_.fetch_add(1, std::memory_order_relaxed);
        } else {
            failed_.fetch_add(1, std::memory_order_relaxed);
        }

        beast::error_code ec;
        stream.socket().shutdown(tcp::socket::shutdown_both, ec);
    } catch (...) {
        failed_.fetch_add(1, std::memory_order_relaxed);
    }
}

TelemetryReporterConfig load_reporter_config_from_env() {
    TelemetryReporterConfig cfg;
    const char* url = std::getenv("TRACK3_INGEST_URL");
    if (!url || std::string(url).empty()) {
        cfg.enabled = false;
        return cfg;
    }
    cfg.enabled = true;

    // Parse "http://host:port" → host + port
    std::string raw(url);
    // Strip scheme
    auto pos = raw.find("://");
    if (pos != std::string::npos) raw = raw.substr(pos + 3);
    // Strip trailing slash
    if (!raw.empty() && raw.back() == '/') raw.pop_back();
    // Split host:port
    auto colon = raw.rfind(':');
    if (colon != std::string::npos) {
        cfg.ingest_host = raw.substr(0, colon);
        cfg.ingest_port = raw.substr(colon + 1);
    } else {
        cfg.ingest_host = raw;
        cfg.ingest_port = "8081";
    }

    const char* run = std::getenv("TRACK3_RUN_ID");
    if (run && run[0]) cfg.run_id = run;

    const char* sub = std::getenv("TRACK3_SUBMISSION_ID");
    if (sub && sub[0]) cfg.submission_id = sub;

    const char* src = std::getenv("TRACK3_SOURCE");
    if (src && src[0]) cfg.source = src;

    return cfg;
}

} // namespace bot_fleet::metrics
