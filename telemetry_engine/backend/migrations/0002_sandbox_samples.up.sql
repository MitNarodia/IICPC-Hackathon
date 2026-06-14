BEGIN;

-- sandbox_samples: Track 1 resource/health telemetry persisted by the stream-
-- processor. One row per probe sample per (run, submission). High-cardinality,
-- time-ordered — a hypertable candidate just like window_aggregates.
CREATE TABLE IF NOT EXISTS sandbox_samples (
    run_id               TEXT             NOT NULL,
    submission_id        TEXT             NOT NULL,
    sample_ts            TIMESTAMPTZ      NOT NULL,
    pod_name             TEXT             NOT NULL DEFAULT '',
    namespace            TEXT             NOT NULL DEFAULT '',
    cpu_millicores       DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_limit_millicores DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_bytes         BIGINT           NOT NULL DEFAULT 0,
    memory_limit_bytes   BIGINT           NOT NULL DEFAULT 0,
    open_fds             INTEGER          NOT NULL DEFAULT 0,
    active_connections   INTEGER          NOT NULL DEFAULT 0,
    health               TEXT             NOT NULL DEFAULT 'READY',
    oom_killed           BOOLEAN          NOT NULL DEFAULT FALSE,
    restart_count        INTEGER          NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_sandbox_samples_lookup
    ON sandbox_samples (run_id, submission_id, sample_ts DESC);

-- TimescaleDB upgrade (optional, same pattern as window_aggregates).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        PERFORM create_hypertable(
            'sandbox_samples', 'sample_ts',
            if_not_exists => TRUE, migrate_data => TRUE);
        PERFORM add_retention_policy(
            'sandbox_samples', INTERVAL '7 days', if_not_exists => TRUE);
    END IF;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'timescaledb setup for sandbox_samples skipped: %', SQLERRM;
END$$;

COMMIT;
