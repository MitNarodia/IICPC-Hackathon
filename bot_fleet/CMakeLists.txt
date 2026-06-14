cmake_minimum_required(VERSION 3.20)
project(bot_fleet VERSION 0.1.0 LANGUAGES CXX)

# set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# --- Dependencies ---
# Works with: vcpkg, system Boost, or manual BOOST_ROOT
if(POLICY CMP0167)
    cmake_policy(SET CMP0167 NEW)
endif()
find_package(Boost 1.80 REQUIRED COMPONENTS system)
find_package(Threads REQUIRED)

# --- Core Library ---
# All production code except the executables' main() lives here so it is
# compiled ONCE and linked verbatim by both the bot_fleet binary and the test
# suite (no recompilation; tests exercise the exact shipped objects).
add_library(bot_fleet_core STATIC
    src/bot/bot.cpp
    src/bot/bot_coordinator.cpp
    src/net/connection.cpp
    src/metrics/hdr_histogram.cpp
    src/metrics/metrics_collector.cpp
    src/metrics/metrics_aggregator.cpp
    src/metrics/telemetry_reporter.cpp
)
target_include_directories(bot_fleet_core PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/src)
target_link_libraries(bot_fleet_core PUBLIC Boost::system Threads::Threads)
# Platform-specific: link ws2_32 on Windows for networking
if(WIN32)
    target_link_libraries(bot_fleet_core PUBLIC ws2_32 wsock32)
endif()

# --- Bot Fleet Executable ---
add_executable(bot_fleet src/main.cpp)
target_link_libraries(bot_fleet PRIVATE bot_fleet_core)

# --- Mock Exchange Server (standalone test target) ---
add_executable(mock_exchange
    mock_server/mock_exchange.cpp
)

target_include_directories(mock_exchange PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/src)
target_link_libraries(mock_exchange PRIVATE Boost::system Threads::Threads)

if(WIN32)
    target_link_libraries(mock_exchange PRIVATE ws2_32 wsock32)
endif()

# --- Tests (GoogleTest) ---
# Toggle off with: cmake -DBOT_FLEET_BUILD_TESTS=OFF
option(BOT_FLEET_BUILD_TESTS "Build the unit and concurrency test suite" ON)
if(BOT_FLEET_BUILD_TESTS)
    enable_testing()
    add_subdirectory(tests)
endif()
