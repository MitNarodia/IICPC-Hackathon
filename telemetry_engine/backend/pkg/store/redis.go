package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis wraps a go-redis client. We use Redis for two things:
//  1. A per-run sorted set (ZSET) keyed by composite score → O(log n) ranking
//     and O(log n + k) top-k reads for the leaderboard.
//  2. A pub/sub channel the leaderboard-service fans out to WebSocket clients.
type Redis struct {
	C *redis.Client
}

// NewRedis dials Redis and pings to confirm reachability.
func NewRedis(ctx context.Context, addr, password string, db int) (*Redis, error) {
	c := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if lastErr = c.Ping(ctx).Err(); lastErr == nil {
			return &Redis{C: c}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second * time.Duration(attempt+1)):
		}
	}
	return nil, fmt.Errorf("connect redis: %w", lastErr)
}

func (r *Redis) Close() error { return r.C.Close() }

// Key helpers — one place to define the Redis key layout.

// LeaderboardKey is the ZSET of submission_id → score for a run.
func LeaderboardKey(runID string) string { return "lb:" + runID + ":scores" }

// EntryKey is the hash holding the full leaderboard entry JSON for a submission.
func EntryKey(runID, submissionID string) string {
	return "lb:" + runID + ":entry:" + submissionID
}

// UpdatesChannel is the pub/sub channel the leaderboard-service subscribes to.
func UpdatesChannel(runID string) string { return "lb:" + runID + ":updates" }

// RunsKey is the set of known run IDs (for the run selector).
const RunsKey = "lb:runs"
