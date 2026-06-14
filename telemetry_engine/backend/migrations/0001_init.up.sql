-- Track 3 schema, migration 0001 (up).
--
-- WHY THIS FILE EXISTS: the analytics tier (stream-processor, validation-engine,
-- scoring-engine) writes its derived results here so the dashboard can query
-- history and the leaderboard survives restarts. Redis holds the HOT board for
-- the WebSocket fan-out; PostgreSQL/TimescaleDB holds the DURABLE, queryable
-- record. The column lists below are the contract the upsert queries in
-- internal/*/persist.go and internal/leaderboard/api.go depend on — keep them
-- in sync.
--
-- TimescaleDB is OPTIONAL: if the extension is present, window_aggregates becomes
-- a hypertable with a retention policy (it is the only high-cardinality, time-
-- series table). On plain PostgreSQL the same schema works as ordinary tables.

BEGIN;

-- ---------------------------------------------------------------------------
-- runs: one row per benchmark run. Multiple runs can be live simultaneously.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS runs (
    run_id     TEXT PRIMARY KEY,
    label      TEXT        NOT NULL DEFAULT '',
    status     TEXT        NOT NULL DEFAULT 'active', -- active | finished | aborted
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- submissions_meta: human-facing identity for a (run, submission) pair, so the
-- leaderboard can show a display name instead of a raw UUID.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS submissions_meta (
    run_id        TEXT NOT NULL,
    submission_id TEXT NOT NULL,
    contestant_id TEXT NOT NULL DEFAULT '',
    display_name  TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, submission_id)
);

-- ---------------------------------------------------------------------------
-- window_aggregates: the stream-processor's per-window output. High volume and
-- time-ordered → the TimescaleDB hypertable. The PRIMARY KEY includes
-- window_start because Timescale requires the partition column in every unique
-- index, and (run, submission, window_start, kind) is genuinely unique.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS window_aggregates (
    run_id        TEXT             NOT NULL,
    submission_id TEXT             NOT NULL,
    window_start  TIMESTAMPTZ      NOT NULL,
    window_end    TIMESTAMPTZ      NOT NULL,
    window_kind   TEXT             NOT NULL, -- tumbling | sliding | rolling
    transactions  BIGINT           NOT NULL DEFAULT 0,
    errors        BIGINT           NOT NULL DEFAULT 0,
    timeouts      BIGINT           NOT NULL DEFAULT 0,
    tps           DOUBLE PRECISION NOT NULL DEFAULT 0,
    error_rate    DOUBLE PRECISION NOT NULL DEFAULT 0,
    p50_us        BIGINT           NOT NULL DEFAULT 0,
    p90_us        BIGINT           NOT NULL DEFAULT 0,
    p99_us        BIGINT           NOT NULL DEFAULT 0,
    max_us        BIGINT           NOT NULL DEFAULT 0,
    mean_us       DOUBLE PRECISION NOT NULL DEFAULT 0,
    sample_count  BIGINT           NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, submission_id, window_start, window_kind)
);

-- Supports the contestant-detail history query:
--   WHERE run_id=? AND submission_id=? AND window_kind='tumbling'
--   ORDER BY window_start DESC
CREATE INDEX IF NOT EXISTS idx_window_agg_lookup
    ON window_aggregates (run_id, submission_id, window_kind, window_start DESC);

-- ---------------------------------------------------------------------------
-- rolling_stats: one continuously-updated row per (run, submission). The
-- authoritative whole-run numbers (peak TPS, stability CoV, final percentiles).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rolling_stats (
    run_id             TEXT             NOT NULL,
    submission_id      TEXT             NOT NULL,
    updated_at         TIMESTAMPTZ      NOT NULL DEFAULT now(),
    total_transactions BIGINT           NOT NULL DEFAULT 0,
    total_errors       BIGINT           NOT NULL DEFAULT 0,
    peak_tps           DOUBLE PRECISION NOT NULL DEFAULT 0,
    current_tps        DOUBLE PRECISION NOT NULL DEFAULT 0,
    p50_us             BIGINT           NOT NULL DEFAULT 0,
    p90_us             BIGINT           NOT NULL DEFAULT 0,
    p99_us             BIGINT           NOT NULL DEFAULT 0,
    error_rate         DOUBLE PRECISION NOT NULL DEFAULT 0,
    tps_stddev         DOUBLE PRECISION NOT NULL DEFAULT 0,
    tps_cov            DOUBLE PRECISION NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, submission_id)
);

-- ---------------------------------------------------------------------------
-- validation_results: the validation-engine's cumulative verdict per submission.
-- violations_by_rule and recent_findings are JSONB so the rule set can evolve
-- without a migration.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS validation_results (
    run_id             TEXT             NOT NULL,
    submission_id      TEXT             NOT NULL,
    updated_at         TIMESTAMPTZ      NOT NULL DEFAULT now(),
    orders_checked     BIGINT           NOT NULL DEFAULT 0,
    trades_checked     BIGINT           NOT NULL DEFAULT 0,
    violations         BIGINT           NOT NULL DEFAULT 0,
    violations_by_rule JSONB            NOT NULL DEFAULT '{}'::jsonb,
    correctness_score  DOUBLE PRECISION NOT NULL DEFAULT 1,
    recent_findings    JSONB            NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (run_id, submission_id)
);

-- ---------------------------------------------------------------------------
-- scores: the scoring-engine's composite output. This is what ranks the board.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS scores (
    run_id            TEXT             NOT NULL,
    submission_id     TEXT             NOT NULL,
    computed_at       TIMESTAMPTZ      NOT NULL DEFAULT now(),
    latency_score     DOUBLE PRECISION NOT NULL DEFAULT 0,
    throughput_score  DOUBLE PRECISION NOT NULL DEFAULT 0,
    correctness_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    stability_score   DOUBLE PRECISION NOT NULL DEFAULT 0,
    composite         DOUBLE PRECISION NOT NULL DEFAULT 0,
    tps               DOUBLE PRECISION NOT NULL DEFAULT 0,
    p50_us            BIGINT           NOT NULL DEFAULT 0,
    p99_us            BIGINT           NOT NULL DEFAULT 0,
    error_rate        DOUBLE PRECISION NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, submission_id)
);

CREATE INDEX IF NOT EXISTS idx_scores_rank
    ON scores (run_id, composite DESC);

-- ---------------------------------------------------------------------------
-- TimescaleDB upgrade (optional). Wrapped so the migration also runs on vanilla
-- PostgreSQL: if the extension isn't available, window_aggregates stays a plain
-- table and everything still works, just without automatic time partitioning.
-- ---------------------------------------------------------------------------
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;
        PERFORM create_hypertable(
            'window_aggregates', 'window_start',
            if_not_exists => TRUE, migrate_data => TRUE);
        -- Keep a week of fine-grained windows; rolling_stats retains the summary.
        PERFORM add_retention_policy(
            'window_aggregates', INTERVAL '7 days', if_not_exists => TRUE);
    END IF;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'timescaledb setup skipped: %', SQLERRM;
END$$;

COMMIT;
