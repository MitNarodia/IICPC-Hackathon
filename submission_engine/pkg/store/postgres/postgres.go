// Package postgres provides a real PostgreSQL implementation of all Track 1
// repository interfaces. It uses pgx/v5 with a connection pool.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/statemachine"
	"github.com/iicpc/track1/submission-engine/pkg/store"
)

// Store implements SubmissionRepository, BuildRepository, DeploymentRepository,
// HealthRepository, and AuditRepository against a PostgreSQL database.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store with a connection pool. It pings the database to
// verify connectivity.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close closes the underlying connection pool.
func (s *Store) Close() { s.pool.Close() }

// Pool returns the raw pool for advanced use cases (migrations, etc.).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// --- SubmissionRepository ---

func (s *Store) CreateSubmission(ctx context.Context, sub *models.Submission) error {
	meta, err := json.Marshal(sub.Metadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO submissions (id, contestant_id, language, submission_type, status,
			entrypoint, declared_port, artifact_uri, artifact_sha256, created_at, updated_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		sub.ID, sub.ContestantID, sub.Language, sub.Type, sub.Status,
		sub.Entrypoint, sub.DeclaredPort, sub.ArtifactURI, sub.ArtifactSHA256,
		sub.CreatedAt, sub.UpdatedAt, meta,
	)
	return err
}

func (s *Store) GetSubmission(ctx context.Context, id string) (*models.Submission, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, contestant_id, language, submission_type, status,
			entrypoint, declared_port, artifact_uri, artifact_sha256,
			created_at, updated_at, metadata
		FROM submissions WHERE id = $1`, id)
	sub := &models.Submission{}
	var meta []byte
	err := row.Scan(&sub.ID, &sub.ContestantID, &sub.Language, &sub.Type, &sub.Status,
		&sub.Entrypoint, &sub.DeclaredPort, &sub.ArtifactURI, &sub.ArtifactSHA256,
		&sub.CreatedAt, &sub.UpdatedAt, &meta)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &sub.Metadata)
	}
	return sub, nil
}

func (s *Store) ListSubmissions(ctx context.Context) ([]*models.Submission, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, contestant_id, language, submission_type, status,
			entrypoint, declared_port, artifact_uri, artifact_sha256,
			created_at, updated_at, metadata
		FROM submissions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.Submission
	for rows.Next() {
		sub := &models.Submission{}
		var meta []byte
		if err := rows.Scan(&sub.ID, &sub.ContestantID, &sub.Language, &sub.Type, &sub.Status,
			&sub.Entrypoint, &sub.DeclaredPort, &sub.ArtifactURI, &sub.ArtifactSHA256,
			&sub.CreatedAt, &sub.UpdatedAt, &meta); err != nil {
			return nil, err
		}
		if len(meta) > 0 {
			_ = json.Unmarshal(meta, &sub.Metadata)
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSubmission(ctx context.Context, sub *models.Submission) error {
	meta, _ := json.Marshal(sub.Metadata)
	sub.UpdatedAt = time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE submissions SET contestant_id=$2, language=$3, submission_type=$4, status=$5,
			entrypoint=$6, declared_port=$7, artifact_uri=$8, artifact_sha256=$9,
			updated_at=$10, metadata=$11
		WHERE id=$1`,
		sub.ID, sub.ContestantID, sub.Language, sub.Type, sub.Status,
		sub.Entrypoint, sub.DeclaredPort, sub.ArtifactURI, sub.ArtifactSHA256,
		sub.UpdatedAt, meta,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// TransitionSubmission atomically transitions the submission status within a DB
// transaction. It uses SELECT … FOR UPDATE to prevent races, computes the new
// state via the state machine, updates the row, and writes an audit log entry.
func (s *Store) TransitionSubmission(ctx context.Context, id string, event statemachine.Event, actor string, detail map[string]interface{}) (*models.Submission, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock the row.
	row := tx.QueryRow(ctx, `
		SELECT id, contestant_id, language, submission_type, status,
			entrypoint, declared_port, artifact_uri, artifact_sha256,
			created_at, updated_at, metadata
		FROM submissions WHERE id = $1 FOR UPDATE`, id)

	sub := &models.Submission{}
	var meta []byte
	if err := row.Scan(&sub.ID, &sub.ContestantID, &sub.Language, &sub.Type, &sub.Status,
		&sub.Entrypoint, &sub.DeclaredPort, &sub.ArtifactURI, &sub.ArtifactSHA256,
		&sub.CreatedAt, &sub.UpdatedAt, &meta); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &sub.Metadata)
	}

	prev := sub.Status
	next, err := statemachine.Transition(prev, event)
	if err != nil {
		return nil, err
	}
	sub.Status = next
	sub.UpdatedAt = time.Now().UTC()

	metaBytes, _ := json.Marshal(sub.Metadata)
	_, err = tx.Exec(ctx, `UPDATE submissions SET status=$2, updated_at=$3, metadata=$4 WHERE id=$1`,
		sub.ID, sub.Status, sub.UpdatedAt, metaBytes)
	if err != nil {
		return nil, err
	}

	// Write audit entry.
	auditID, err := models.NewUUIDv7()
	if err != nil {
		return nil, err
	}
	detailJSON, _ := json.Marshal(detail)
	if detail == nil {
		detailJSON = []byte("{}")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_log (id, submission_id, actor, action, prev_state, new_state, detail, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		auditID, id, actor, string(event), string(prev), string(next), detailJSON, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return sub, nil
}

// --- BuildRepository ---

func (s *Store) SaveBuild(ctx context.Context, build *models.Build) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO builds (id, submission_id, status, image_ref, image_digest, build_logs_uri, started_at, finished_at, error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET status=$3, image_ref=$4, image_digest=$5,
			build_logs_uri=$6, started_at=$7, finished_at=$8, error=$9`,
		build.ID, build.SubmissionID, build.Status, build.ImageRef, build.ImageDigest,
		build.LogsURI, build.StartedAt, build.FinishedAt, build.Error,
	)
	return err
}

func (s *Store) GetBuild(ctx context.Context, submissionID string) (*models.Build, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, submission_id, status, image_ref, image_digest, build_logs_uri, started_at, finished_at, error
		FROM builds WHERE submission_id=$1 ORDER BY started_at DESC LIMIT 1`, submissionID)
	b := &models.Build{}
	err := row.Scan(&b.ID, &b.SubmissionID, &b.Status, &b.ImageRef, &b.ImageDigest,
		&b.LogsURI, &b.StartedAt, &b.FinishedAt, &b.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	return b, err
}

// --- DeploymentRepository ---

func (s *Store) SaveDeployment(ctx context.Context, d *models.Deployment) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO deployments (id, submission_id, status, pod_name, namespace, node_name, cpu_cores, memory_mb, runtime_class, created_at, terminated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET status=$3, pod_name=$4, node_name=$6,
			terminated_at=$11`,
		d.ID, d.SubmissionID, d.Status, d.PodName, d.Namespace, d.NodeName,
		d.CPUCores, d.MemoryMB, d.RuntimeClass, d.CreatedAt, d.TerminatedAt,
	)
	return err
}

func (s *Store) GetDeployment(ctx context.Context, submissionID string) (*models.Deployment, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, submission_id, status, pod_name, namespace, node_name, cpu_cores, memory_mb, runtime_class, created_at, terminated_at
		FROM deployments WHERE submission_id=$1 ORDER BY created_at DESC LIMIT 1`, submissionID)
	d := &models.Deployment{}
	err := row.Scan(&d.ID, &d.SubmissionID, &d.Status, &d.PodName, &d.Namespace,
		&d.NodeName, &d.CPUCores, &d.MemoryMB, &d.RuntimeClass, &d.CreatedAt, &d.TerminatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	return d, err
}

func (s *Store) SaveEndpoint(ctx context.Context, ep *models.Endpoint) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO endpoints (id, submission_id, deployment_id, internal_url, service_name, protocol, status, registered_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET status=$7`,
		ep.ID, ep.SubmissionID, ep.DeploymentID, ep.InternalURL, ep.ServiceName,
		ep.Protocol, ep.Status, ep.RegisteredAt,
	)
	return err
}

func (s *Store) GetEndpoint(ctx context.Context, submissionID string) (*models.Endpoint, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, submission_id, deployment_id, internal_url, service_name, protocol, status, registered_at
		FROM endpoints WHERE submission_id=$1 ORDER BY registered_at DESC LIMIT 1`, submissionID)
	ep := &models.Endpoint{}
	err := row.Scan(&ep.ID, &ep.SubmissionID, &ep.DeploymentID, &ep.InternalURL,
		&ep.ServiceName, &ep.Protocol, &ep.Status, &ep.RegisteredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	return ep, err
}

func (s *Store) SetEndpointStatus(ctx context.Context, submissionID string, status models.EndpointStatus) error {
	tag, err := s.pool.Exec(ctx, `UPDATE endpoints SET status=$2 WHERE submission_id=$1`, submissionID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// --- HealthRepository ---

func (s *Store) AddHealthSample(ctx context.Context, sample models.HealthSample) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO health_samples (time, submission_id, deployment_id, healthy, latency_ms, cpu_pct, mem_mb, restarts)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		sample.Time, sample.SubmissionID, sample.DeploymentID,
		sample.Healthy, sample.LatencyMS, sample.CPUPct, sample.MemMB, sample.Restarts,
	)
	return err
}

func (s *Store) ListHealthSamples(ctx context.Context, submissionID string, limit int) ([]models.HealthSample, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT time, submission_id, deployment_id, healthy, latency_ms, cpu_pct, mem_mb, restarts
		FROM health_samples WHERE submission_id=$1 ORDER BY time DESC LIMIT $2`, submissionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.HealthSample
	for rows.Next() {
		var h models.HealthSample
		if err := rows.Scan(&h.Time, &h.SubmissionID, &h.DeploymentID,
			&h.Healthy, &h.LatencyMS, &h.CPUPct, &h.MemMB, &h.Restarts); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// --- AuditRepository ---

func (s *Store) WriteAudit(ctx context.Context, entry models.AuditLog) error {
	detail, _ := json.Marshal(entry.Detail)
	if entry.Detail == nil {
		detail = []byte("{}")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_log (id, submission_id, actor, action, prev_state, new_state, detail, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		entry.ID, entry.SubmissionID, entry.Actor, entry.Action,
		entry.PrevState, entry.NewState, detail, entry.CreatedAt,
	)
	return err
}

func (s *Store) ListAudit(ctx context.Context, submissionID string) ([]models.AuditLog, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, submission_id, actor, action, prev_state, new_state, detail, created_at
		FROM audit_log WHERE submission_id=$1 ORDER BY created_at DESC`, submissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AuditLog
	for rows.Next() {
		var a models.AuditLog
		var detail []byte
		if err := rows.Scan(&a.ID, &a.SubmissionID, &a.Actor, &a.Action,
			&a.PrevState, &a.NewState, &detail, &a.CreatedAt); err != nil {
			return nil, err
		}
		if len(detail) > 0 {
			_ = json.Unmarshal(detail, &a.Detail)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
