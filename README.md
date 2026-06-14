# RUNBOOK — Full-Stack Deployment

---

## Required Software

| Tool         | Minimum Version | Notes                          |
|--------------|-----------------|--------------------------------|
| Go           | 1.22            | Track 1 & Track 3 backends    |
| C++ compiler | GCC 13 / Clang 17 | Track 2 (C++20 required)    |
| CMake        | 3.25            | Track 2 build system           |
| vcpkg        | latest          | Track 2 dependency manager     |
| Docker       | 24.0+           | Container runtime              |
| Docker Compose | 2.20+         | Stack orchestration            |
| Node.js      | 20 LTS          | Track 3 frontend               |
| Make         | any             | Track 1 convenience targets    |
| protoc       | 3.25+           | Proto code generation          |
| buf          | 1.28+           | Protobuf management            |

---

## Environment Variables

### Track 1 (submission_engine)

```bash
# Postgres
DATABASE_URL=postgres://track1:track1@localhost:5432/submissions?sslmode=disable

# Redis
REDIS_ADDR=localhost:6379

# S3 / MinIO
S3_ENDPOINT=http://localhost:9000
S3_BUCKET=submissions
AWS_ACCESS_KEY_ID=minioadmin
AWS_SECRET_ACCESS_KEY=minioadmin

# Kafka / Redpanda
KAFKA_BROKERS=localhost:9092

# Docker orchestrator
DOCKER_HOST=                         # empty = default socket
SANDBOX_NETWORK=track1-sandbox-net
BUILD_TIMEOUT_SECONDS=600
SANDBOX_CPU=1
SANDBOX_MEMORY_MB=512

# Track 3 integration
TRACK3_INGEST_URL=http://localhost:8081
TRACK3_RUN_ID=run-001

# Service ports
SUBMISSION_API_PORT=8080
UPLOAD_SERVICE_PORT=8082
BUILD_SERVICE_PORT=8083
DEPLOYMENT_MANAGER_PORT=8084
HEALTH_MONITOR_PORT=8085
SANDBOX_RUNNER_PORT=8086
```

### Track 2 (bot_fleet)

```bash
EXCHANGE_HOST=localhost
EXCHANGE_PORT=9001
BOT_COUNT=50
SHARD_COUNT=4
TICK_MS=100

# Track 3 integration
TRACK3_INGEST_URL=http://localhost:8081
TRACK3_RUN_ID=run-001
TRACK3_SUBMISSION_ID=bot-fleet-001
TRACK3_SOURCE=bot-fleet
```

### Track 3 (telemetry_engine)

```bash
# TimescaleDB
TIMESCALE_URL=postgres://track3:track3@localhost:5433/telemetry?sslmode=disable

# Redis
REDIS_ADDR=localhost:6380

# Redpanda
KAFKA_BROKERS=localhost:9093

# Ports
INGESTION_PORT=8081
STREAM_PROCESSOR_PORT=8091
VALIDATION_PORT=8092
SCORING_PORT=8093
LEADERBOARD_PORT=8094
```

---

## Build Commands

### Track 1

```bash
cd submission_engine
make proto        # generate protobuf Go code
make build        # compile all services
```

Or individually:

```bash
cd services/submission-api && go build -o ../../bin/submission-api ./cmd/submission-api
cd services/upload-service && go build -o ../../bin/upload-service ./cmd/upload-service
cd services/build-service && go build -o ../../bin/build-service ./cmd/build-service
cd services/deployment-manager && go build -o ../../bin/deployment-manager ./cmd/deployment-manager
cd services/health-monitor && go build -o ../../bin/health-monitor ./cmd/health-monitor
cd services/sandbox-runner && go build -o ../../bin/sandbox-runner ./cmd/sandbox-runner
```

### Track 2

```bash
cd bot_fleet
cmake --preset default    # or: cmake -B build -DCMAKE_TOOLCHAIN_FILE=$VCPKG_ROOT/scripts/buildsystems/vcpkg.cmake
cmake --build build --config Release -j$(nproc)
```

### Track 3

```bash
cd telemetry_engine
make build        # compile all services
cd frontend && npm ci && npm run build
```

---

## Startup Order

Infrastructure must start before application services.

### Phase 1 — Infrastructure

```bash
docker compose up -d postgres redis minio redpanda timescaledb
```

Wait for healthy checks (~10s):

```bash
docker compose exec postgres pg_isready
docker compose exec redis redis-cli ping
docker compose exec timescaledb pg_isready -p 5433
```

### Phase 2 — Migrations

```bash
cd submission_engine && make migrate-up
cd telemetry_engine && make migrate-up
```

### Phase 3 — Track 3 Services (telemetry must be up before Track 1/2 report)

```bash
# In order:
./bin/ingestion-service &
./bin/stream-processor &
./bin/validation-engine &
./bin/scoring-engine &
./bin/leaderboard-service &
```

### Phase 4 — Track 1 Services

```bash
./bin/submission-api &
./bin/upload-service &
./bin/build-service &
./bin/deployment-manager &
./bin/health-monitor &
./bin/sandbox-runner &
```

### Phase 5 — Track 2

```bash
cd bot_fleet/build
./bot_fleet
```

### Phase 6 — Track 3 Frontend

```bash
cd telemetry_engine/frontend
npm run preview   # or: npm run dev
```

---

## Deployment Order (Docker Compose — single host)

```bash
# Full stack
docker compose up -d

# Or by profile:
docker compose --profile infra up -d
docker compose --profile track3 up -d
docker compose --profile track1 up -d
docker compose --profile track2 up -d
docker compose --profile frontend up -d
```

---

## Verification Commands

```bash
# Track 1 health
curl -s http://localhost:8080/healthz   # submission-api
curl -s http://localhost:8085/healthz   # health-monitor

# Track 3 health
curl -s http://localhost:8081/healthz   # ingestion

# Track 2 → Track 3 integration
curl -s http://localhost:8081/v1/track2/bot-metrics   # returns 405 (POST only) = alive

# Track 1 → Track 3 integration
curl -s http://localhost:8081/v1/track1/sandbox       # returns 405 (POST only) = alive

# Track 1 deployment discovery (new)
curl -s http://localhost:8080/v1/submissions/<id>/deployment | jq .

# WebSocket stream
wscat -c ws://localhost:8081/v1/ws

# Track 1 submission flow (e2e)
curl -X POST http://localhost:8080/v1/submissions \
  -H "Content-Type: application/json" \
  -d '{"language":"go","team_id":"team-1"}'

# Track 3 leaderboard
curl -s http://localhost:8094/v1/leaderboard
```

---

## End-to-End Demo

### Automated (recommended)

```bash
# From repository root — runs the full Track 1 → Track 2 → Track 3 pipeline:
bash scripts/run_integration.sh
```

See `scripts/run_integration.sh` for configurable environment variables.

### Manual Steps

```bash
# 1. Start full stack
docker compose up -d

# 2. Submit a solution
SUBMISSION_ID=$(curl -s -X POST http://localhost:8080/v1/submissions \
  -H "Content-Type: application/json" \
  -d '{"language":"go","team_id":"demo-team"}' | jq -r '.id')

# 3. Upload source
curl -X PUT "http://localhost:8082/v1/uploads/${SUBMISSION_ID}" \
  -F "file=@test/samples/hello-go/main.go"

# 4. Wait for deployment & discover endpoint
watch -n2 "curl -s http://localhost:8080/v1/submissions/${SUBMISSION_ID}/deployment | jq ."
# Wait until status == "READY" and note the endpoint URL.

# 5. Run bot fleet against the deployed contestant (auto-resolve)
TRACK3_INGEST_URL=http://localhost:8081 \
TRACK3_SUBMISSION_ID=${SUBMISSION_ID} \
./bot_fleet/build/bot_fleet \
  --track1-api http://localhost:8080 \
  --submission-id ${SUBMISSION_ID} \
  --bots 50 --orders 20

# 6. Or run bot fleet standalone against mock_exchange (unchanged)
cd bot_fleet/build && ./bot_fleet --host 127.0.0.1 --port 9091

# 7. Monitor pipeline progression in Track 3
wscat -c ws://localhost:8081/v1/ws

# 8. Check leaderboard after scoring completes
curl -s http://localhost:8094/v1/leaderboard | jq .
```

---

## Shutdown Commands

```bash
# Graceful shutdown (reverse order)
kill %bot_fleet 2>/dev/null
docker compose down

# Force cleanup (removes volumes)
docker compose down -v --remove-orphans

# Remove sandbox network
docker network rm track1-sandbox-net 2>/dev/null
```

---

## Troubleshooting Quick Reference

| Symptom | Fix |
|---------|-----|
| `orchestrator wiring: ping` | Docker daemon not running |
| `kafka: dial tcp: connection refused` | Redpanda not ready; wait or check `docker compose logs redpanda` |
| Track 2 no metrics in Track 3 | Check `TRACK3_INGEST_URL` is set for bot_fleet |
| Health-monitor not reporting | Check `TRACK3_INGEST_URL` + `TRACK3_RUN_ID` env vars |
| Seccomp blocks syscall | Review `deploy/seccomp/sandbox-default.json` and add missing syscall |
