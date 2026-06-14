# tests/CMakeLists.txt
# ====================
# GoogleTest-based unit + concurrency suite for the Bot Fleet.
# Links against bot_fleet_core so it tests the exact production objects.
#
# Run with:   ctest --test-dir build --output-on-failure
# Or directly: ./build/tests/bot_fleet_tests --gtest_filter='SeqLock*'
#
# For the concurrency suite it is strongly recommended to also build a
# ThreadSanitizer variant, e.g.:
#   cmake -B build-tsan -DCMAKE_CXX_FLAGS="-fsanitize=thread -O1 -g"
#   cmake --build build-tsan --target bot_fleet_tests
#   ./build-tsan/tests/bot_fleet_tests --gtest_filter='*Concurren*:SeqLock*'

find_package(GTest CONFIG REQUIRED)
include(GoogleTest)

add_executable(bot_fleet_tests
    unit/test_seqlock.cpp
    unit/test_hdr_histogram.cpp
    unit/test_metrics_aggregation.cpp
    unit/test_market_simulator.cpp
    unit/test_persona_generation.cpp
    unit/test_order_generation.cpp
    concurrency/test_seqlock_torn_reads.cpp
    concurrency/test_clean_shutdown.cpp
    concurrency/test_shard_isolation.cpp
    concurrency/test_concurrent_metrics.cpp
)

target_link_libraries(bot_fleet_tests PRIVATE
    bot_fleet_core
    GTest::gtest
    GTest::gtest_main
)

# Lets tests include "support/..." and "bot/...", "metrics/...", etc.
target_include_directories(bot_fleet_tests PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}
    ${CMAKE_SOURCE_DIR}/src
)

# Auto-register every TEST() with CTest (enables `ctest`, sharding, filtering).
gtest_discover_tests(bot_fleet_tests)
