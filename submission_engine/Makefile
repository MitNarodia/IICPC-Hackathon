GO ?= go
BUILD_OUTPUT_DIR ?= /private/tmp/track1-build-bin

.PHONY: test build fmt vet migrate-up migrate-down tidy up down clean logs e2e

tidy: ## Run go mod tidy
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

build:
	@mkdir -p $(BUILD_OUTPUT_DIR)
	@set -e; for svc in services/*/cmd/*/main.go; do \
		svc_dir=$$(dirname $$(dirname $$(dirname $$svc))); \
		name=$$(basename $$svc_dir); \
		echo "==> build $$name"; \
		mkdir -p $(BUILD_OUTPUT_DIR)/$$name; \
		$(GO) build -o $(BUILD_OUTPUT_DIR)/$$name/ ./$$svc_dir/cmd/...; \
	done

migrate-up:
	@if ! command -v migrate >/dev/null 2>&1; then \
		echo "golang-migrate CLI is required"; exit 1; \
	fi
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is required"; exit 1)
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	@if ! command -v migrate >/dev/null 2>&1; then \
		echo "golang-migrate CLI is required"; exit 1; \
	fi
	@test -n "$(DATABASE_URL)" || (echo "DATABASE_URL is required"; exit 1)
	migrate -path migrations -database "$(DATABASE_URL)" down 1

## --- Docker Compose targets ---

.PHONY: up down clean logs e2e

up: ## Bring up the full stack (infra + services)
	docker compose up --build -d

down: ## Stop the stack (keep volumes)
	docker compose down

clean: ## Stop and wipe all volumes
	docker compose down -v

logs: ## Tail all service logs
	docker compose logs -f

e2e: ## Run end-to-end pipeline test (stack must be up)
	@echo "Waiting 10s for services to stabilize..."
	@sleep 10
	@echo "Creating a test submission..."
	curl -s -X POST http://localhost:8080/v1/submissions \
		-H "Content-Type: application/json" \
		-d '{"contestant_id":"01912345-6789-7abc-8def-0123456789ab","language":"cpp","submission_type":"source","entrypoint":"./bot","declared_port":8081}' | python3 -m json.tool
	@echo "\nCheck status after 5s..."
	@sleep 5
	curl -s http://localhost:8080/v1/submissions | python3 -m json.tool
