# Track 3 — Telemetry, Validation, Analytics & Real-Time Leaderboard
# Developer task runner. `make help` lists everything.
#
# Most targets assume Docker + a Go 1.22 toolchain. The full stack needs only
# Docker: `make up`.

SHELL        := bash
BACKEND      := backend
FRONTEND     := frontend
SERVICES     := ingestion-service stream-processor validation-engine scoring-engine leaderboard-service
IMAGE_PREFIX ?= track3
IMAGE_TAG    ?= latest

# Local connection defaults (override on the CLI: `make migrate-up POSTGRES_DSN=...`).
POSTGRES_DSN ?= postgres://track3:track3@localhost:5432/track3?sslmode=disable
INGEST_URL   ?= http://localhost:8081
BOARD_URL    ?= http://localhost:8080

.DEFAULT_GOAL := help

## ----------------------------------------------------------------------------
## Help
## ----------------------------------------------------------------------------
.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

## ----------------------------------------------------------------------------
## Go backend
## ----------------------------------------------------------------------------
.PHONY: tidy
tidy: ## go mod tidy (also writes go.sum)
	cd $(BACKEND) && go mod tidy

.PHONY: build
build: ## Build every service binary into backend/bin/
	cd $(BACKEND) && mkdir -p bin && \
	for s in $(SERVICES) telemetry-simulator; do \
		echo "building $$s"; \
		go build -o bin/$$s ./cmd/$$s || exit 1; \
	done

.PHONY: test
test: ## Run unit tests
	cd $(BACKEND) && go test ./...

.PHONY: test-race
test-race: ## Run unit tests with the race detector
	cd $(BACKEND) && go test -race ./...

.PHONY: test-integration
test-integration: ## Run end-to-end tests against a running stack (make up first)
	cd $(BACKEND) && INGEST_URL=$(INGEST_URL) LEADERBOARD_URL=$(BOARD_URL) \
		go test -tags=integration -count=1 ./test/integration/...

.PHONY: vet
vet: ## go vet
	cd $(BACKEND) && go vet ./...

.PHONY: fmt
fmt: ## Format Go code
	cd $(BACKEND) && gofmt -s -w .

.PHONY: lint
lint: vet ## Format-check + vet (CI gate)
	cd $(BACKEND) && test -z "$$(gofmt -s -l . | tee /dev/stderr)"

## ----------------------------------------------------------------------------
## Database
## ----------------------------------------------------------------------------
.PHONY: migrate-up
migrate-up: ## Apply schema to POSTGRES_DSN
	psql "$(POSTGRES_DSN)" -v ON_ERROR_STOP=1 -f $(BACKEND)/migrations/0001_init.up.sql

.PHONY: migrate-down
migrate-down: ## Drop schema from POSTGRES_DSN
	psql "$(POSTGRES_DSN)" -v ON_ERROR_STOP=1 -f $(BACKEND)/migrations/0001_init.down.sql

## ----------------------------------------------------------------------------
## Docker / local stack
## ----------------------------------------------------------------------------
.PHONY: up
up: ## Bring the whole stack up (build + run) via docker compose
	docker compose up --build -d

.PHONY: down
down: ## Stop the stack (keep volumes)
	docker compose down

.PHONY: clean
clean: ## Stop the stack and delete volumes
	docker compose down -v

.PHONY: logs
logs: ## Tail all service logs
	docker compose logs -f

.PHONY: docker-build
docker-build: ## Build all service images with the ARG SERVICE Dockerfile
	@for s in $(SERVICES); do \
		echo "==> $(IMAGE_PREFIX)/$$s:$(IMAGE_TAG)"; \
		docker build --build-arg SERVICE=$$s -t $(IMAGE_PREFIX)/$$s:$(IMAGE_TAG) ./$(BACKEND) || exit 1; \
	done
	docker build -t $(IMAGE_PREFIX)/frontend:$(IMAGE_TAG) ./$(FRONTEND)

## ----------------------------------------------------------------------------
## Frontend
## ----------------------------------------------------------------------------
.PHONY: web-install
web-install: ## Install frontend deps
	cd $(FRONTEND) && npm install

.PHONY: web-dev
web-dev: ## Run the dashboard dev server (proxies /v1 to :8080)
	cd $(FRONTEND) && npm run dev

.PHONY: web-build
web-build: ## Production build of the dashboard
	cd $(FRONTEND) && npm run build

## ----------------------------------------------------------------------------
## Demo & load
## ----------------------------------------------------------------------------
.PHONY: simulate
simulate: ## Stream synthetic contestants at the running stack
	cd $(BACKEND) && go run ./cmd/telemetry-simulator -url $(INGEST_URL) -subs 8 -eps 400 -duration 5m

.PHONY: load
load: ## k6 load test against ingestion (needs k6 installed)
	INGEST_URL=$(INGEST_URL) k6 run $(BACKEND)/test/load/ingest_load.js

## ----------------------------------------------------------------------------
## Kubernetes / Helm
## ----------------------------------------------------------------------------
.PHONY: k8s-config
k8s-config: ## Create the migrations ConfigMap the k8s migrate Job needs
	kubectl create configmap track3-migrations \
		--from-file=$(BACKEND)/migrations/0001_init.up.sql -n track3 \
		--dry-run=client -o yaml | kubectl apply -f -

.PHONY: k8s-apply
k8s-apply: ## Apply raw manifests in dependency order
	kubectl apply -f infra/k8s/namespace.yaml
	kubectl apply -f infra/k8s/config.yaml
	kubectl apply -f infra/k8s/infra.yaml
	$(MAKE) k8s-config
	kubectl apply -f infra/k8s/migrate-job.yaml
	kubectl apply -f infra/k8s/services.yaml
	kubectl apply -f infra/k8s/ingress.yaml
	kubectl apply -f infra/k8s/hpa.yaml

.PHONY: helm-install
helm-install: ## Install/upgrade the Helm chart
	helm upgrade --install track3 infra/helm/track3 -n track3 --create-namespace
