# Bot Fleet Makefile
# Requires: Docker (for containerized build) OR cmake + vcpkg (for local build)

.PHONY: build run test bench clean docker-build up down logs help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-15s %s\n", $$1, $$2}'

# ─── Docker Targets (preferred — no local toolchain needed) ───────────────

docker-build: ## Build the Docker image (compiles everything inside container)
	docker compose build

up: ## Start mock exchange + bot fleet (default: 1000 bots, 100 orders each)
	docker compose up --build

down: ## Stop everything
	docker compose down

test: ## Run unit + concurrency tests
	docker compose run --rm tests

bench: ## Run full benchmark suite (100/1k/5k/10k bots)
	docker compose run --rm benchmark

logs: ## Tail logs
	docker compose logs -f

# ─── Local Build Targets (requires cmake, vcpkg, boost) ──────────────────

VCPKG_ROOT ?= $(HOME)/vcpkg
BUILD_DIR ?= build

local-build: ## Build locally with cmake + vcpkg
	cmake -B $(BUILD_DIR) -G Ninja \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_TOOLCHAIN_FILE=$(VCPKG_ROOT)/scripts/buildsystems/vcpkg.cmake \
		-DBOT_FLEET_BUILD_TESTS=ON
	cmake --build $(BUILD_DIR) --parallel

local-test: local-build ## Run tests locally
	ctest --test-dir $(BUILD_DIR) --output-on-failure

local-run: local-build ## Run bot fleet against local mock exchange
	@echo "Starting mock exchange on port 9090..."
	@$(BUILD_DIR)/mock_exchange 9090 &
	@sleep 1
	$(BUILD_DIR)/bot_fleet --host 127.0.0.1 --port 9090 --bots 1000 --conns 50 --orders 100
	@kill %1 2>/dev/null || true

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)
	docker compose down --rmi local -v 2>/dev/null || true
