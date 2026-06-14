// Package redisreg implements store.Registry backed by Redis.
package redisreg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/iicpc/track1/submission-engine/pkg/models"
	"github.com/iicpc/track1/submission-engine/pkg/store"
)

const (
	keyPrefix    = "endpoint:"
	activeSetKey = "endpoints:active"
)

// Registry implements store.Registry against Redis.
type Registry struct {
	client *redis.Client
}

// New creates a Redis-backed registry and pings the server.
func New(ctx context.Context, redisURL string) (*Registry, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redisreg: parse url: %w", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redisreg: ping: %w", err)
	}
	return &Registry{client: client}, nil
}

// Close closes the Redis client.
func (r *Registry) Close() error { return r.client.Close() }

func (r *Registry) RegisterEndpoint(ctx context.Context, endpoint models.Endpoint) error {
	data, err := json.Marshal(endpoint)
	if err != nil {
		return err
	}
	key := keyPrefix + endpoint.SubmissionID
	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, activeSetKey, endpoint.SubmissionID)
	_, err = pipe.Exec(ctx)
	return err
}

func (r *Registry) DeregisterEndpoint(ctx context.Context, submissionID string) error {
	key := keyPrefix + submissionID
	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, activeSetKey, submissionID)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *Registry) LookupEndpoint(ctx context.Context, submissionID string) (*models.Endpoint, error) {
	key := keyPrefix + submissionID
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var ep models.Endpoint
	if err := json.Unmarshal(data, &ep); err != nil {
		return nil, err
	}
	return &ep, nil
}
