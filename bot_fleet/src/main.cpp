/// main.cpp
/// ========
/// WHY THIS FILE EXISTS:
///   Entry point. Parses command-line arguments, constructs the RunConfig,
///   and delegates to BotCoordinator. Kept minimal — all logic lives in
///   the coordinator and its sub-components.
///
/// MODES OF OPERATION:
///   1. Standalone (default): targets --host/--port directly (e.g. mock_exchange)
///   2. Direct URL:    --target-url ws://host:port
///   3. Track 1 auto:  --track1-api http://... --submission-id <uuid>
///      Polls the Track 1 deployment endpoint until READY, then extracts
///      the deployed contestant's host:port.
///
/// THREADING MODEL:
///   main() runs on the main thread. After calling coordinator.execute(),
///   that same thread becomes the io_context event loop thread.
///   Total process threads: 1 (plus OS/runtime housekeeping threads).
///
/// QUEUES: None.
/// ASYNC OPS: None directly — all async work is inside the coordinator.

#include "bot/bot_coordinator.hpp"
#include <boost/asio/connect.hpp>
#include <boost/beast/core.hpp>
#include <boost/beast/http.hpp>
#include <boost/beast/version.hpp>
#include <iostream>
#include <string>
#include <cstdlib>
#include <thread>
#include <chrono>

namespace {

namespace net = boost::asio;
namespace beast = boost::beast;
namespace http = beast::http;
using tcp = net::ip::tcp;

/// Parse a URL like "ws://host:port" or "http://host:port" into host + port.
/// Returns true on success.
bool parse_url(const std::string& url, std::string& host, std::string& port,
               const std::string& default_port) {
    std::string raw = url;
    // Strip scheme
    auto pos = raw.find("://");
    if (pos != std::string::npos) raw = raw.substr(pos + 3);
    // Strip trailing slash
    if (!raw.empty() && raw.back() == '/') raw.pop_back();
    // Strip path
    auto slash = raw.find('/');
    if (slash != std::string::npos) raw = raw.substr(0, slash);
    // Split host:port
    auto colon = raw.rfind(':');
    if (colon != std::string::npos) {
        host = raw.substr(0, colon);
        port = raw.substr(colon + 1);
    } else {
        host = raw;
        port = default_port;
    }
    return !host.empty();
}

/// Call GET {track1_api}/v1/submissions/{submission_id}/deployment and return
/// the JSON response body. Returns empty string on failure.
std::string fetch_deployment(const std::string& api_host, const std::string& api_port,
                             const std::string& submission_id) {
    try {
        net::io_context ioc;
        tcp::resolver resolver(ioc);
        beast::tcp_stream stream(ioc);

        auto results = resolver.resolve(api_host, api_port);
        stream.connect(results);
        stream.expires_after(std::chrono::seconds(5));

        std::string target = "/v1/submissions/" + submission_id + "/deployment";
        http::request<http::empty_body> req(http::verb::get, target, 11);
        req.set(http::field::host, api_host);
        req.set(http::field::user_agent, "BotFleet/0.1");
        req.set(http::field::accept, "application/json");

        http::write(stream, req);

        beast::flat_buffer buffer;
        http::response<http::string_body> res;
        http::read(stream, buffer, res);

        beast::error_code ec;
        stream.socket().shutdown(tcp::socket::shutdown_both, ec);

        if (res.result_int() >= 200 && res.result_int() < 300) {
            return res.body();
        }
        std::cerr << "[resolve] HTTP " << res.result_int() << ": " << res.body() << "\n";
        return "";
    } catch (const std::exception& e) {
        std::cerr << "[resolve] Error: " << e.what() << "\n";
        return "";
    }
}

/// Minimal JSON string extractor: finds "key":"value" and returns value.
/// Does NOT handle escaped quotes inside values (sufficient for our use case).
std::string json_string_value(const std::string& json, const std::string& key) {
    std::string needle = "\"" + key + "\":\"";
    auto pos = json.find(needle);
    if (pos == std::string::npos) return "";
    auto start = pos + needle.size();
    auto end = json.find('"', start);
    if (end == std::string::npos) return "";
    return json.substr(start, end - start);
}

/// Resolve a Track 1 deployment endpoint by polling the API.
/// Returns true if the endpoint was resolved and host/port were set.
bool resolve_track1_endpoint(const std::string& track1_api,
                             const std::string& submission_id,
                             std::string& out_host,
                             std::string& out_port) {
    std::string api_host, api_port;
    if (!parse_url(track1_api, api_host, api_port, "8080")) {
        std::cerr << "[resolve] Cannot parse Track 1 API URL: " << track1_api << "\n";
        return false;
    }

    std::cout << "[resolve] Resolving deployment for submission " << submission_id << "\n";
    std::cout << "[resolve] Track 1 API: " << api_host << ":" << api_port << "\n";

    constexpr int max_attempts = 60;
    constexpr int poll_interval_ms = 2000;

    for (int attempt = 1; attempt <= max_attempts; ++attempt) {
        std::string body = fetch_deployment(api_host, api_port, submission_id);
        if (body.empty()) {
            std::cerr << "[resolve] Attempt " << attempt << "/" << max_attempts
                      << " — API unreachable, retrying...\n";
            std::this_thread::sleep_for(std::chrono::milliseconds(poll_interval_ms));
            continue;
        }

        std::string status = json_string_value(body, "status");
        std::string endpoint = json_string_value(body, "endpoint");

        std::cout << "[resolve] Attempt " << attempt << "/" << max_attempts
                  << " — status=" << status;
        if (!endpoint.empty()) std::cout << ", endpoint=" << endpoint;
        std::cout << "\n";

        if (status == "READY" && !endpoint.empty()) {
            // Parse the endpoint URL to extract host:port
            if (parse_url(endpoint, out_host, out_port, "8081")) {
                std::cout << "[resolve] ✓ Deployment ready: " << out_host << ":" << out_port << "\n";
                return true;
            }
            std::cerr << "[resolve] Cannot parse endpoint URL: " << endpoint << "\n";
            return false;
        }

        if (status == "FAILED" || status == "TERMINATED") {
            std::cerr << "[resolve] Deployment " << status << " — aborting.\n";
            return false;
        }

        std::this_thread::sleep_for(std::chrono::milliseconds(poll_interval_ms));
    }

    std::cerr << "[resolve] Timed out after " << max_attempts << " attempts.\n";
    return false;
}

} // anonymous namespace

void print_usage(const char* prog) {
    std::cout << "Usage: " << prog << " [options]\n"
              << "\n"
              << "Modes:\n"
              << "  Standalone (default)  Target a host:port directly (e.g. mock_exchange)\n"
              << "  --target-url <url>    Target a WebSocket URL (e.g. ws://host:port)\n"
              << "  --track1-api <url>    Auto-resolve from Track 1 (requires --submission-id)\n"
              << "\n"
              << "Standalone options:\n"
              << "  --host <addr>         Target host (default: 127.0.0.1)\n"
              << "  --port <port>         Target port (default: 9090)\n"
              << "\n"
              << "Integration options:\n"
              << "  --target-url <url>    Direct WebSocket URL (e.g. ws://contestant:8081)\n"
              << "  --track1-api <url>    Track 1 API base URL (e.g. http://localhost:8080)\n"
              << "  --submission-id <id>  Submission ID to resolve (requires --track1-api)\n"
              << "\n"
              << "Load options:\n"
              << "  --bots <n>            Number of bots (default: 1000)\n"
              << "  --conns <n>           Connection pool size (default: 50)\n"
              << "  --workers <n>         Worker threads/cores (default: hardware concurrency)\n"
              << "  --orders <n>          Orders per bot (default: 100)\n"
              << "  --seed <n>            Random seed (default: 12345)\n"
              << "\n"
              << "Telemetry (Track 3) env vars:\n"
              << "  TRACK3_INGEST_URL     Track 3 ingestion URL (e.g. http://localhost:8081)\n"
              << "  TRACK3_RUN_ID         Benchmark run identifier\n"
              << "  TRACK3_SUBMISSION_ID  Submission ID for telemetry tagging\n"
              << "  TRACK3_SOURCE         Source label (default: bot-fleet)\n"
              << "\n"
              << "  --help                Show this message\n";
}

int main(int argc, char* argv[]) {
    bot_fleet::bot::RunConfig config;

    // Integration mode variables
    std::string target_url;
    std::string track1_api;
    std::string submission_id;

    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];
        if (arg == "--help") {
            print_usage(argv[0]);
            return 0;
        } else if (arg == "--host" && i + 1 < argc) {
            config.target_host = argv[++i];
        } else if (arg == "--port" && i + 1 < argc) {
            config.target_port = argv[++i];
        } else if (arg == "--bots" && i + 1 < argc) {
            config.num_bots = static_cast<uint32_t>(std::stoul(argv[++i]));
        } else if (arg == "--conns" && i + 1 < argc) {
            config.connection_pool_size = static_cast<uint32_t>(std::stoul(argv[++i]));
        } else if (arg == "--workers" && i + 1 < argc) {
            config.num_workers = static_cast<unsigned>(std::stoul(argv[++i]));
        } else if (arg == "--orders" && i + 1 < argc) {
            config.orders_per_bot = std::stoull(argv[++i]);
        } else if (arg == "--seed" && i + 1 < argc) {
            config.run_seed = std::stoull(argv[++i]);
        } else if (arg == "--target-url" && i + 1 < argc) {
            target_url = argv[++i];
        } else if (arg == "--track1-api" && i + 1 < argc) {
            track1_api = argv[++i];
        } else if (arg == "--submission-id" && i + 1 < argc) {
            submission_id = argv[++i];
        } else {
            std::cerr << "Unknown option: " << arg << "\n";
            print_usage(argv[0]);
            return 1;
        }
    }

    // --- Resolve target endpoint ---

    if (!track1_api.empty() && !submission_id.empty()) {
        // Mode: Track 1 auto-resolve
        std::string resolved_host, resolved_port;
        if (!resolve_track1_endpoint(track1_api, submission_id,
                                     resolved_host, resolved_port)) {
            std::cerr << "[FATAL] Could not resolve deployment endpoint from Track 1.\n";
            return 1;
        }
        config.target_host = resolved_host;
        config.target_port = resolved_port;
    } else if (!track1_api.empty() || !submission_id.empty()) {
        std::cerr << "[FATAL] Both --track1-api and --submission-id are required together.\n";
        return 1;
    } else if (!target_url.empty()) {
        // Mode: Direct URL
        std::string parsed_host, parsed_port;
        if (!parse_url(target_url, parsed_host, parsed_port, "9090")) {
            std::cerr << "[FATAL] Cannot parse --target-url: " << target_url << "\n";
            return 1;
        }
        config.target_host = parsed_host;
        config.target_port = parsed_port;
    }
    // else: standalone mode — use --host/--port defaults (127.0.0.1:9090)

    try {
        bot_fleet::bot::BotCoordinator coordinator(config);
        coordinator.execute();
    } catch (const std::exception& e) {
        std::cerr << "[FATAL] " << e.what() << "\n";
        return 1;
    }

    return 0;
}
