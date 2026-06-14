#!/usr/bin/env bash
#
# scripts/run_benchmarks.sh
# =========================
# Scalability benchmark harness for the Bot Fleet load generator.
#
# WHAT IT DOES
#   For each scale (100, 1k, 5k, 10k bots) it:
#     1. launches the bundled mock_exchange (once, shared across scales),
#     2. runs ./bot_fleet against it with a scale-appropriate connection pool,
#     3. samples CPU / peak-RSS / open-FDs / live-connections once per second
#        while the run is in flight,
#     4. parses the fleet's "FINAL CUMULATIVE" report for TPS and p50/p90/p99,
#     5. writes one row per scale to a CSV and a structured JSON document.
#
# OUTPUTS (under results/run_<timestamp>/)
#   summary.csv   - one row per scale, stable column order (spreadsheet / pandas)
#   summary.json  - schema-versioned document for Track-3 ingestion (see below)
#   fleet_<scale>.log / sample_<scale>.csv / mock_exchange.log - raw artefacts
#   results/latest.json + results/latest.csv - convenience copies for ingestion
#
# CONFIG (environment overrides)
#   BIN_DIR=build           directory containing bot_fleet + mock_exchange
#   HOST=127.0.0.1  PORT=9090
#   ORDERS=200              orders per bot (run length)
#   WORKERS=0               0 => binary auto-selects hardware_concurrency
#   SAMPLE_INTERVAL=1       seconds between resource samples
#   OUT_DIR=results/run_... override the output directory
#
# REQUIREMENTS (Linux): bash, ps, ss (iproute2), awk, grep -P (GNU), /proc.
#
# IMPORTANT CAVEAT
#   The bundled mock_exchange is single-threaded. At ~10k bots it can saturate
#   one core and become the latency bottleneck (not the fleet). To measure pure
#   FLEET scaling, run several mock instances on separate cores/ports and point
#   one bot_fleet process at each. This script intentionally keeps a single
#   mock for a reproducible, self-contained baseline and warns at 10k.
#
set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
HOST="${HOST:-127.0.0.1}"
PORT="${PORT:-9090}"
BIN_DIR="${BIN_DIR:-build}"
ORDERS="${ORDERS:-200}"
WORKERS="${WORKERS:-0}"
SAMPLE_INTERVAL="${SAMPLE_INTERVAL:-1}"

# Scale matrix: bots and the connection-pool size to use for each.
SCALE_BOTS=(100 1000 5000 10000)
SCALE_CONNS=(50 200 500 1000)
SCALE_LABEL=("100bots" "1k_bots" "5k_bots" "10k_bots")

FLEET_BIN="$BIN_DIR/bot_fleet"
MOCK_BIN="$BIN_DIR/mock_exchange"

TS="$(date +%Y%m%d_%H%M%S)"
OUT_DIR="${OUT_DIR:-results/run_$TS}"
mkdir -p "$OUT_DIR"
CSV="$OUT_DIR/summary.csv"
JSON="$OUT_DIR/summary.json"

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------
[[ -x "$FLEET_BIN" ]] || { echo "ERROR: $FLEET_BIN not found/executable. Build first: cmake --build $BIN_DIR"; exit 1; }
[[ -x "$MOCK_BIN"  ]] || { echo "ERROR: $MOCK_BIN not found/executable.";  exit 1; }
command -v ss  >/dev/null 2>&1 || echo "WARN: 'ss' not found; connection counts will be 0."
command -v ps  >/dev/null 2>&1 || { echo "ERROR: 'ps' is required."; exit 1; }

MOCK_PID=""
SAMPLER_PID=""
cleanup() {
    [[ -n "${SAMPLER_PID}" ]] && kill "${SAMPLER_PID}" 2>/dev/null || true
    [[ -n "${MOCK_PID}"    ]] && kill "${MOCK_PID}"    2>/dev/null || true
}
trap cleanup EXIT INT TERM

# ---------------------------------------------------------------------------
# Start the mock exchange (shared across all scales)
# ---------------------------------------------------------------------------
echo "[bench] starting mock_exchange on ${HOST}:${PORT}"
"$MOCK_BIN" "$PORT" > "$OUT_DIR/mock_exchange.log" 2>&1 &
MOCK_PID=$!

# Wait until it is actually listening (up to 5s).
for _ in $(seq 1 50); do
    if ss -ltn 2>/dev/null | grep -q ":${PORT} "; then break; fi
    sleep 0.1
done
kill -0 "$MOCK_PID" 2>/dev/null || { echo "ERROR: mock_exchange failed to start (see $OUT_DIR/mock_exchange.log)"; exit 1; }

# ---------------------------------------------------------------------------
# CSV header (stable column order — safe for Track-3 schema mapping)
# ---------------------------------------------------------------------------
echo "timestamp,scale_label,bots,workers,conns,orders_per_bot,duration_s,transactions,errors,tps,p50_us,p90_us,p99_us,cpu_avg_pct,cpu_peak_pct,rss_peak_kb,fds_peak,conns_peak" > "$CSV"

# Background resource sampler: $1=pid to watch, $2=output csv.
sample_proc() {
    local pid="$1" out="$2"
    echo "t,cpu_pct,rss_peak_kb,fds,conns" > "$out"
    while kill -0 "$pid" 2>/dev/null; do
        local cpu rss fds conns
        cpu=$(ps -p "$pid" -o %cpu= 2>/dev/null | tr -d ' ');               cpu=${cpu:-0}
        rss=$(awk '/VmHWM/{print $2}' "/proc/$pid/status" 2>/dev/null);     rss=${rss:-0}
        fds=$(ls "/proc/$pid/fd" 2>/dev/null | wc -l);                      fds=${fds:-0}
        conns=$(ss -tan state established "( dport = :$PORT )" 2>/dev/null | tail -n +2 | wc -l); conns=${conns:-0}
        echo "$(date +%s),${cpu},${rss},${fds},${conns}" >> "$out"
        sleep "$SAMPLE_INTERVAL"
    done
}

RUN_JSON=()

# ---------------------------------------------------------------------------
# Run each scale
# ---------------------------------------------------------------------------
for i in "${!SCALE_BOTS[@]}"; do
    bots="${SCALE_BOTS[$i]}"
    conns="${SCALE_CONNS[$i]}"
    label="${SCALE_LABEL[$i]}"
    fleet_log="$OUT_DIR/fleet_${label}.log"
    sample_csv="$OUT_DIR/sample_${label}.csv"

    echo "[bench] scale=${label}  bots=${bots}  conns=${conns}  orders=${ORDERS}"

    wflag=()
    [[ "$WORKERS" != "0" ]] && wflag=(--workers "$WORKERS")

    "$FLEET_BIN" --host "$HOST" --port "$PORT" \
        --bots "$bots" --conns "$conns" --orders "$ORDERS" "${wflag[@]}" \
        > "$fleet_log" 2>&1 &
    fleet_pid=$!

    sample_proc "$fleet_pid" "$sample_csv" &
    SAMPLER_PID=$!

    wait "$fleet_pid" || true
    kill "$SAMPLER_PID" 2>/dev/null || true
    wait "$SAMPLER_PID" 2>/dev/null || true
    SAMPLER_PID=""

    # --- parse the FINAL CUMULATIVE block from the fleet log ---
    final="$(awk '/FINAL CUMULATIVE/{c=1} c{print}' "$fleet_log")"
    getnum() { printf '%s\n' "$final" | grep -oP "$1" | head -1 || true; }

    tps=$(getnum 'Aggregate TPS:\s*\K[0-9.]+');     tps=${tps:-0}
    txns=$(getnum 'Transactions:\s*\K[0-9]+');      txns=${txns:-0}
    errors=$(getnum 'Errors:\s*\K[0-9]+');          errors=${errors:-0}
    dur=$(getnum 'Window \(s\):\s*\K[0-9.]+');      dur=${dur:-0}
    p50=$(getnum 'p50=\K[0-9]+');                   p50=${p50:-0}
    p90=$(getnum 'p90=\K[0-9]+');                   p90=${p90:-0}
    p99=$(getnum 'p99=\K[0-9]+');                   p99=${p99:-0}
    workers_used=$(grep -oP 'Workers \(cores\):\s*\K[0-9]+' "$fleet_log" | head -1 || true)
    workers_used=${workers_used:-0}

    # --- aggregate the resource sampler (avg/peak CPU, peak RSS/FDs/conns) ---
    read -r cpu_avg cpu_peak rss_peak fds_peak conns_peak < <(
        awk -F, 'NR>1{
            n++; cs+=$2;
            if($2>cp)cp=$2; if($3>rp)rp=$3; if($4>fp)fp=$4; if($5>kp)kp=$5
        } END{ printf "%.1f %.1f %d %d %d", (n? cs/n : 0), cp+0, rp+0, fp+0, kp+0 }' "$sample_csv"
    )

    ts_now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "${ts_now},${label},${bots},${workers_used},${conns},${ORDERS},${dur},${txns},${errors},${tps},${p50},${p90},${p99},${cpu_avg},${cpu_peak},${rss_peak},${fds_peak},${conns_peak}" >> "$CSV"

    RUN_JSON+=("$(cat <<EOF
    {
      "scale": "${label}",
      "bots": ${bots},
      "workers": ${workers_used},
      "conns": ${conns},
      "orders_per_bot": ${ORDERS},
      "duration_s": ${dur},
      "transactions": ${txns},
      "errors": ${errors},
      "tps": ${tps},
      "latency_us": { "p50": ${p50}, "p90": ${p90}, "p99": ${p99} },
      "cpu_pct": { "avg": ${cpu_avg}, "peak": ${cpu_peak} },
      "memory_kb": { "rss_peak": ${rss_peak} },
      "fds_peak": ${fds_peak},
      "conns_peak": ${conns_peak}
    }
EOF
    )")

    if [[ "$bots" -ge 10000 ]]; then
        echo "[bench] NOTE: single-threaded mock_exchange may be the bottleneck at ${bots} bots."
        echo "        For pure fleet scaling, shard the mock across cores/ports."
    fi
done

# ---------------------------------------------------------------------------
# Assemble the Track-3 ingestion JSON document
# ---------------------------------------------------------------------------
git_sha="$(git -C "$(dirname "$0")/.." rev-parse --short HEAD 2>/dev/null || echo unknown)"

runs_joined=""
for idx in "${!RUN_JSON[@]}"; do
    runs_joined+="${RUN_JSON[$idx]}"
    [[ "$idx" -lt $(( ${#RUN_JSON[@]} - 1 )) ]] && runs_joined+=","
    runs_joined+=$'\n'
done

cat > "$JSON" <<EOF
{
  "schema_version": "1.0",
  "benchmark": "bot_fleet_load_generator",
  "generated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "host": {
    "hostname": "$(hostname)",
    "cpu_count": $(nproc 2>/dev/null || echo 0),
    "kernel": "$(uname -sr)"
  },
  "target": { "host": "${HOST}", "port": ${PORT} },
  "binary": { "bot_fleet": "${FLEET_BIN}", "git_sha": "${git_sha}" },
  "config": { "orders_per_bot": ${ORDERS}, "sample_interval_s": ${SAMPLE_INTERVAL} },
  "runs": [
${runs_joined}  ]
}
EOF

mkdir -p results
cp "$JSON" results/latest.json
cp "$CSV"  results/latest.csv

echo "[bench] done."
echo "[bench] CSV : $CSV"
echo "[bench] JSON: $JSON"
echo "[bench] Track-3 copies: results/latest.json, results/latest.csv"
echo
column -s, -t "$CSV" 2>/dev/null || cat "$CSV"
