#!/usr/bin/env bash
# =============================================================================
# scripts/run_integration.sh
# =============================================================================
# End-to-end integration demo: Track 1 → Track 2 → Track 3
#
# This script:
#   1. Submits contestant code to Track 1
#   2. Waits for deployment to become READY
#   3. Discovers the deployed endpoint
#   4. Starts bot_fleet against that endpoint with Track 3 telemetry enabled
#   5. Queries Track 3 leaderboard to verify the full pipeline
#
# All ports and URLs are configurable via environment variables.
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration (all overridable via environment variables)
# ---------------------------------------------------------------------------

# Track 1
TRACK1_API_URL="${TRACK1_API_URL:-http://localhost:8080}"
UPLOAD_SERVICE_URL="${UPLOAD_SERVICE_URL:-http://localhost:8082}"

# Track 3
TRACK3_INGEST_URL="${TRACK3_INGEST_URL:-http://localhost:8081}"
TRACK3_LEADERBOARD_URL="${TRACK3_LEADERBOARD_URL:-http://localhost:8094}"
TRACK3_RUN_ID="${TRACK3_RUN_ID:-integration-$(date +%s)}"

# Bot fleet
BOT_FLEET_BINARY="${BOT_FLEET_BINARY:-./bot_fleet/build/bot_fleet}"
BOT_COUNT="${BOT_COUNT:-50}"
BOT_ORDERS="${BOT_ORDERS:-20}"
BOT_CONNS="${BOT_CONNS:-10}"

# Submission parameters
CONTESTANT_ID="${CONTESTANT_ID:-01912345-6789-7abc-8def-0123456789ab}"
CONTESTANT_LANGUAGE="${CONTESTANT_LANGUAGE:-go}"
CONTESTANT_PORT="${CONTESTANT_PORT:-8081}"

# Polling
POLL_INTERVAL="${POLL_INTERVAL:-2}"
POLL_MAX_ATTEMPTS="${POLL_MAX_ATTEMPTS:-60}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# ---------------------------------------------------------------------------
# Helper functions
# ---------------------------------------------------------------------------

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

check_dependency() {
    command -v "$1" >/dev/null 2>&1 || fail "Required tool '$1' not found. Please install it."
}

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------

info "═══════════════════════════════════════════════════════════════"
info " Track 1 → Track 2 → Track 3  Integration Demo"
info "═══════════════════════════════════════════════════════════════"
echo ""

check_dependency curl
check_dependency jq

# Check Track 1 is reachable
info "Checking Track 1 API at ${TRACK1_API_URL}..."
if curl -sf "${TRACK1_API_URL}/healthz" > /dev/null 2>&1; then
    ok "Track 1 API is reachable."
else
    fail "Track 1 API at ${TRACK1_API_URL}/healthz is not responding.\n       Make sure the submission-api service is running."
fi

# Check Track 3 (optional — warn only)
info "Checking Track 3 ingestion at ${TRACK3_INGEST_URL}..."
if curl -sf "${TRACK3_INGEST_URL}/healthz" > /dev/null 2>&1; then
    ok "Track 3 ingestion is reachable."
else
    warn "Track 3 ingestion not reachable. Telemetry will be disabled."
fi

echo ""

# ---------------------------------------------------------------------------
# Step 1: Submit contestant code to Track 1
# ---------------------------------------------------------------------------

info "Step 1/5: Creating submission..."

SUBMISSION_RESPONSE=$(curl -sf -X POST "${TRACK1_API_URL}/v1/submissions" \
    -H "Content-Type: application/json" \
    -d "{
        \"contestant_id\": \"${CONTESTANT_ID}\",
        \"language\": \"${CONTESTANT_LANGUAGE}\",
        \"submission_type\": \"source\",
        \"entrypoint\": \"./bot\",
        \"declared_port\": ${CONTESTANT_PORT}
    }" 2>&1) || fail "Failed to create submission.\n       Response: ${SUBMISSION_RESPONSE}"

SUBMISSION_ID=$(echo "${SUBMISSION_RESPONSE}" | jq -r '.id // empty')
UPLOAD_URL=$(echo "${SUBMISSION_RESPONSE}" | jq -r '.upload_url // empty')

if [ -z "${SUBMISSION_ID}" ]; then
    fail "No submission ID in response: ${SUBMISSION_RESPONSE}"
fi

ok "Submission created: ${SUBMISSION_ID}"
info "Upload URL: ${UPLOAD_URL}"
echo ""

# ---------------------------------------------------------------------------
# Step 2: Wait for deployment to become READY
# ---------------------------------------------------------------------------

info "Step 2/5: Waiting for deployment to become READY..."
info "Polling GET ${TRACK1_API_URL}/v1/submissions/${SUBMISSION_ID}/deployment"
info "(max ${POLL_MAX_ATTEMPTS} attempts, ${POLL_INTERVAL}s interval)"
echo ""

ENDPOINT=""
for attempt in $(seq 1 ${POLL_MAX_ATTEMPTS}); do
    DEPLOY_RESPONSE=$(curl -sf "${TRACK1_API_URL}/v1/submissions/${SUBMISSION_ID}/deployment" 2>&1) || true

    if [ -z "${DEPLOY_RESPONSE}" ]; then
        echo "  Attempt ${attempt}/${POLL_MAX_ATTEMPTS} — API not responding, retrying..."
        sleep "${POLL_INTERVAL}"
        continue
    fi

    STATUS=$(echo "${DEPLOY_RESPONSE}" | jq -r '.status // "UNKNOWN"')
    ENDPOINT=$(echo "${DEPLOY_RESPONSE}" | jq -r '.endpoint // empty')

    echo "  Attempt ${attempt}/${POLL_MAX_ATTEMPTS} — status=${STATUS}"

    if [ "${STATUS}" = "READY" ] && [ -n "${ENDPOINT}" ]; then
        ok "Deployment is READY!"
        ok "Endpoint: ${ENDPOINT}"
        echo ""
        break
    fi

    if [ "${STATUS}" = "FAILED" ] || [ "${STATUS}" = "TERMINATED" ]; then
        fail "Deployment ${STATUS}. Check Track 1 logs for details."
    fi

    sleep "${POLL_INTERVAL}"
done

if [ -z "${ENDPOINT}" ]; then
    fail "Deployment did not become READY within ${POLL_MAX_ATTEMPTS} attempts."
fi

# ---------------------------------------------------------------------------
# Step 3: Discover the deployed endpoint
# ---------------------------------------------------------------------------

info "Step 3/5: Parsing endpoint..."

# Extract host and port from the endpoint URL
ENDPOINT_HOST=$(echo "${ENDPOINT}" | sed -E 's|^https?://||; s|:[0-9]+$||; s|/.*||')
ENDPOINT_PORT=$(echo "${ENDPOINT}" | grep -oP ':\K[0-9]+(?=/|$)' || echo "8081")

ok "Target: ${ENDPOINT_HOST}:${ENDPOINT_PORT}"
echo ""

# ---------------------------------------------------------------------------
# Step 4: Start bot_fleet against the deployed endpoint
# ---------------------------------------------------------------------------

info "Step 4/5: Starting bot fleet load test..."
info "Binary: ${BOT_FLEET_BINARY}"
info "Target: ${ENDPOINT_HOST}:${ENDPOINT_PORT}"
info "Bots: ${BOT_COUNT}, Orders/bot: ${BOT_ORDERS}, Connections: ${BOT_CONNS}"
echo ""

if [ ! -f "${BOT_FLEET_BINARY}" ]; then
    warn "Bot fleet binary not found at ${BOT_FLEET_BINARY}"
    info "Trying to run via --track1-api mode..."

    # Alternative: use the Track 1 API auto-resolve mode
    export TRACK3_INGEST_URL
    export TRACK3_RUN_ID
    export TRACK3_SUBMISSION_ID="${SUBMISSION_ID}"
    export TRACK3_SOURCE="bot-fleet"

    info "Would run: ${BOT_FLEET_BINARY} --track1-api ${TRACK1_API_URL} --submission-id ${SUBMISSION_ID} --bots ${BOT_COUNT} --orders ${BOT_ORDERS} --conns ${BOT_CONNS}"
    warn "Skipping bot fleet execution (binary not found)."
else
    export TRACK3_INGEST_URL
    export TRACK3_RUN_ID
    export TRACK3_SUBMISSION_ID="${SUBMISSION_ID}"
    export TRACK3_SOURCE="bot-fleet"

    "${BOT_FLEET_BINARY}" \
        --host "${ENDPOINT_HOST}" \
        --port "${ENDPOINT_PORT}" \
        --bots "${BOT_COUNT}" \
        --orders "${BOT_ORDERS}" \
        --conns "${BOT_CONNS}" \
        || warn "Bot fleet exited with non-zero status (may be expected if sandbox is not a real WebSocket server)"
fi

echo ""

# ---------------------------------------------------------------------------
# Step 5: Verify Track 3 leaderboard
# ---------------------------------------------------------------------------

info "Step 5/5: Checking Track 3 leaderboard..."

LEADERBOARD=$(curl -sf "${TRACK3_LEADERBOARD_URL}/v1/leaderboard" 2>&1) || true

if [ -n "${LEADERBOARD}" ]; then
    ok "Leaderboard response received:"
    echo "${LEADERBOARD}" | jq . 2>/dev/null || echo "${LEADERBOARD}"
else
    warn "Could not fetch leaderboard from ${TRACK3_LEADERBOARD_URL}."
    warn "Track 3 may not be running."
fi

echo ""
info "═══════════════════════════════════════════════════════════════"
ok   " Integration demo complete!"
info "═══════════════════════════════════════════════════════════════"
echo ""
info "Summary:"
info "  Submission ID:  ${SUBMISSION_ID}"
info "  Endpoint:       ${ENDPOINT}"
info "  Track 3 Run ID: ${TRACK3_RUN_ID}"
echo ""
info "To run again with the same submission:"
info "  ${BOT_FLEET_BINARY} --track1-api ${TRACK1_API_URL} --submission-id ${SUBMISSION_ID}"
echo ""
info "To tear down the deployment:"
info "  curl -X POST ${TRACK1_API_URL}/v1/submissions/${SUBMISSION_ID}/teardown"
echo ""
