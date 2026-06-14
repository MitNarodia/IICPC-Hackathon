CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE submissions (
    id uuid PRIMARY KEY,
    contestant_id uuid NOT NULL,
    language text NOT NULL CHECK (language IN ('cpp', 'rust', 'go')),
    submission_type text NOT NULL CHECK (submission_type IN ('source', 'binary')),
    status text NOT NULL CHECK (status IN (
        'CREATED','UPLOADED','VALIDATING','VALIDATED','BUILDING','BUILT',
        'SCANNING','SCANNED','DEPLOYING','HEALTH_CHECK','READY','DEGRADED',
        'TERMINATED','UPLOAD_FAILED','VALIDATION_FAILED','BUILD_FAILED',
        'SCAN_FAILED','DEPLOY_FAILED','HEALTH_FAILED'
    )),
    entrypoint text NOT NULL,
    declared_port integer NOT NULL CHECK (declared_port BETWEEN 1024 AND 65535),
    artifact_uri text,
    artifact_sha256 text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX submissions_contestant_id_idx ON submissions (contestant_id);
CREATE INDEX submissions_status_idx ON submissions (status);
CREATE INDEX submissions_created_at_idx ON submissions (created_at);

CREATE TABLE builds (
    id uuid PRIMARY KEY,
    submission_id uuid NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('PENDING','RUNNING','SUCCESS','FAILED')),
    image_ref text,
    image_digest text,
    build_logs_uri text,
    started_at timestamptz,
    finished_at timestamptz,
    error text
);

CREATE TABLE scans (
    id uuid PRIMARY KEY,
    submission_id uuid NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    image_digest text NOT NULL,
    status text NOT NULL CHECK (status IN ('PENDING','RUNNING','PASSED','FAILED')),
    findings jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE deployments (
    id uuid PRIMARY KEY,
    submission_id uuid NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('PENDING','SCHEDULING','RUNNING','READY','DEGRADED','TERMINATED','FAILED')),
    pod_name text,
    namespace text NOT NULL DEFAULT 'track1-sandbox',
    node_name text,
    cpu_cores integer NOT NULL CHECK (cpu_cores > 0),
    memory_mb integer NOT NULL CHECK (memory_mb > 0),
    runtime_class text NOT NULL DEFAULT 'gvisor',
    created_at timestamptz NOT NULL DEFAULT now(),
    terminated_at timestamptz
);

CREATE TABLE endpoints (
    id uuid PRIMARY KEY,
    submission_id uuid NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    internal_url text NOT NULL,
    service_name text NOT NULL,
    protocol text NOT NULL CHECK (protocol IN ('http','ws','grpc')),
    status text NOT NULL CHECK (status IN ('ACTIVE','INACTIVE')),
    registered_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE health_samples (
    time timestamptz NOT NULL,
    submission_id uuid NOT NULL,
    deployment_id uuid NOT NULL,
    healthy boolean NOT NULL,
    latency_ms double precision NOT NULL,
    cpu_pct double precision NOT NULL,
    mem_mb double precision NOT NULL,
    restarts integer NOT NULL DEFAULT 0
);

SELECT create_hypertable('health_samples', 'time', if_not_exists => TRUE);

CREATE TABLE audit_log (
    id uuid PRIMARY KEY,
    submission_id uuid,
    actor text NOT NULL,
    action text NOT NULL,
    prev_state text,
    new_state text,
    detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION reject_audit_log_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$;

CREATE TRIGGER audit_log_no_update
BEFORE UPDATE ON audit_log
FOR EACH ROW EXECUTE FUNCTION reject_audit_log_mutation();

CREATE TRIGGER audit_log_no_delete
BEFORE DELETE ON audit_log
FOR EACH ROW EXECUTE FUNCTION reject_audit_log_mutation();
