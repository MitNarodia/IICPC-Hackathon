#!/bin/bash
set -e

echo "🚀 Starting End-to-End Bot Integration Test..."

# 1. Generate UUID
UUID=$(cat /proc/sys/kernel/random/uuid)
echo "[-] Generated Contestant ID: $UUID"

# 2. Create Submission
echo "[-] Creating submission with Track 1..."
SUBMISSION_RESPONSE=$(curl -s -X POST http://localhost:8080/v1/submissions \
  -H "Content-Type: application/json" \
  -d "{
    \"contestant_id\":\"$UUID\",
    \"language\":\"go\",
    \"submission_type\":\"source\",
    \"entrypoint\":\"./bot\",
    \"declared_port\":8081
  }")

SUBMISSION_ID=$(echo "$SUBMISSION_RESPONSE" | jq -r '.id')
UPLOAD_URL=$(echo "$SUBMISSION_RESPONSE" | jq -r '.upload_url' | sed 's/\\u0026/\&/g')

echo "[-] Submission ID: $SUBMISSION_ID"

# 3. Upload Artifact
echo "[-] Uploading websocket bot to S3..."
curl -s -o /dev/null -X PUT "$UPLOAD_URL" \
  --resolve minio:9000:127.0.0.1 \
  --upload-file ./submission_engine/test/samples/go-websocket/bot.tar.gz

echo "[-] Artifact successfully uploaded!"

# 4. Wait for READY
echo -n "[-] Waiting for sandbox deployment (compiling and deploying)"
while true; do
  STATUS=$(curl -s http://localhost:8080/v1/submissions/${SUBMISSION_ID}/deployment | jq -r '.status')
  if [ "$STATUS" = "READY" ]; then
    echo ""
    echo "[-] Deployment is READY! Endpoint is alive."
    break
  elif [ "$STATUS" = "FAILED" ]; then
    echo ""
    echo "[!] Deployment FAILED! Check submission engine logs."
    exit 1
  fi
  echo -n "."
  sleep 2
done

# 5. Start Bot Fleet with Telemetry
echo ""
echo "========================================================="
echo "📊 VIEW LEADERBOARD NOW: http://localhost:8088/"
echo "📈 VIEW GRAFANA METRICS: http://localhost:3000/ (admin/admin)"
echo "========================================================="
echo "[-] Starting Bot Fleet Load Generator..."

cd bot_fleet
docker compose run --rm --no-deps \
  -e TRACK3_INGEST_URL=http://track3-ingestion-service-1:8080 \
  -e TRACK3_SUBMISSION_ID=$SUBMISSION_ID \
  -e TRACK3_RUN_ID="demo-run-${UUID:0:8}" \
  bot_fleet \
  --track1-api http://track1-submission-api-1:8080 \
  --submission-id $SUBMISSION_ID \
  --bots 50 --orders 20

echo "✅ Test completed successfully!"
