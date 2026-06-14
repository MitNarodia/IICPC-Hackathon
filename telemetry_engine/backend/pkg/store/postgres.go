// Package store holds the persistence clients: PostgreSQL/TimescaleDB for
// durable, queryable history and Redis for the hot leaderboard state that the
// WebSocket fan-out reads on every tick.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres wraps a pgx connection pool. TimescaleDB is just Postgres with the
// timescaledb extension loaded, so the same client serves both the relational
// tables and the hypertables.
type Postgres struct {
	Pool *pgxpool.Pool
}

// NewPostgres opens a pool and verifies connectivity with a ping. Retries a few
// times so a service started before the DB is ready (compose race) recovers.
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 16
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour

	var pool *pgxpool.Pool
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		pool, lastErr = pgxpool.NewWithConfig(ctx, cfg)
		if lastErr == nil {
			if lastErr = pool.Ping(ctx); lastErr == nil {
				return &Postgres{Pool: pool}, nil
			}
			pool.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second * time.Duration(attempt+1)):
		}
	}
	return nil, fmt.Errorf("connect postgres: %w", lastErr)
}

func (p *Postgres) Close() { p.Pool.Close() }
