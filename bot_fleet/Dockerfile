# Bot Fleet — Multi-stage build with vcpkg
# Produces two binaries: bot_fleet + mock_exchange
FROM ubuntu:24.04 AS build

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential cmake ninja-build git curl zip unzip tar pkg-config \
    ca-certificates python3 && \
    rm -rf /var/lib/apt/lists/*

# Install vcpkg
ENV VCPKG_ROOT=/opt/vcpkg
RUN git clone --depth 1 https://github.com/microsoft/vcpkg.git $VCPKG_ROOT && \
    $VCPKG_ROOT/bootstrap-vcpkg.sh -disableMetrics

WORKDIR /src
COPY vcpkg.json CMakeLists.txt ./
COPY src/ src/
COPY mock_server/ mock_server/
COPY tests/ tests/

# Configure + build (Release, vcpkg toolchain)
RUN cmake -B build -G Ninja \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_TOOLCHAIN_FILE=$VCPKG_ROOT/scripts/buildsystems/vcpkg.cmake \
    -DBOT_FLEET_BUILD_TESTS=ON && \
    cmake --build build --parallel $(nproc)

# --- Runtime image (minimal) ---
FROM ubuntu:24.04 AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
    libstdc++6 ca-certificates bash iproute2 procps gawk curl && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /src/build/bot_fleet /usr/local/bin/bot_fleet
COPY --from=build /src/build/mock_exchange /usr/local/bin/mock_exchange
COPY --from=build /src/build/tests/bot_fleet_tests /usr/local/bin/bot_fleet_tests
COPY scripts/ /opt/scripts/

ENTRYPOINT ["/usr/local/bin/bot_fleet"]
