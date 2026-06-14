package leaderboard

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	"github.com/iicpc/track3/telemetry-engine/pkg/models"
	"github.com/iicpc/track3/telemetry-engine/pkg/store"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
	"github.com/redis/go-redis/v9"
)

// board is one run's in-memory ranking state, guarded by its own lock so runs
// never contend with each other.
type board struct {
	mu      sync.RWMutex
	entries map[string]*models.LeaderboardEntry // submission_id -> entry
}

func newBoard() *board { return &board{entries: make(map[string]*models.LeaderboardEntry)} }

// Service consumes Score events and maintains the live, ranked board for every
// run. It is the read model behind both the REST API and the WebSocket hub.
//
// REPLICA NOTE: each leaderboard-service instance joins Kafka with a UNIQUE
// consumer group, so every replica sees the full score stream and can serve any
// client from local memory. Redis is the durable mirror (for REST across
// restarts and the run registry), not the hot path.
type Service struct {
	hub   *Hub
	redis *store.Redis
	log   *logpkg.Logger

	mu    sync.RWMutex
	runs  map[string]*board
	order []string // run ids in first-seen order (for the selector)
}

// NewService builds the read model.
func NewService(hub *Hub, redis *store.Redis, log *logpkg.Logger) *Service {
	return &Service{
		hub:   hub,
		redis: redis,
		log:   log,
		runs:  make(map[string]*board),
	}
}

// boardFor returns (creating if needed) the board for a run.
func (s *Service) boardFor(runID string) *board {
	s.mu.RLock()
	b := s.runs[runID]
	s.mu.RUnlock()
	if b != nil {
		return b
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if b = s.runs[runID]; b == nil {
		b = newBoard()
		s.runs[runID] = b
		s.order = append(s.order, runID)
	}
	return b
}

// Consume is the run loop: read scores, fold them into the board, broadcast.
// The scoring engine publishes plain models.Score JSON keyed by run:submission.
func (s *Service) Consume(ctx context.Context, c *kafka.Consumer) error {
	for {
		msg, err := c.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.log.Error("score read", logpkg.F("err", err.Error()))
			continue
		}
		var sc models.Score
		if json.Unmarshal(msg.Value, &sc) == nil && sc.RunID != "" && sc.SubmissionID != "" {
			s.applyScore(ctx, sc)
		}
		_ = c.Commit(ctx, msg)
	}
}

// applyScore folds one score into a run's board, recomputes ranks, mirrors to
// Redis, and broadcasts the new snapshot to WebSocket subscribers.
func (s *Service) applyScore(ctx context.Context, sc models.Score) {
	b := s.boardFor(sc.RunID)

	b.mu.Lock()
	e := b.entries[sc.SubmissionID]
	if e == nil {
		e = &models.LeaderboardEntry{
			RunID:        sc.RunID,
			SubmissionID: sc.SubmissionID,
			Health:       "READY",
		}
		b.entries[sc.SubmissionID] = e
	}
	if sc.DisplayName != "" {
		e.DisplayName = sc.DisplayName
	} else if e.DisplayName == "" {
		e.DisplayName = shortID(sc.SubmissionID)
	}
	e.Composite = sc.Composite
	e.LatencyScore = sc.LatencyScore
	e.ThroughputScore = sc.ThroughputScore
	e.CorrectnessScore = sc.CorrectnessScore
	e.StabilityScore = sc.StabilityScore
	e.TPS = sc.TPS
	e.P50US = sc.P50US
	e.P99US = sc.P99US
	e.ErrorRate = sc.ErrorRate
	if !sc.ComputedAt.IsZero() {
		e.UpdatedAt = sc.ComputedAt
	} else {
		e.UpdatedAt = time.Now().UTC()
	}

	ordered := b.sortedLocked()
	for i, en := range ordered {
		en.PrevRank = en.Rank // remember where it was before this update
		en.Rank = i + 1
	}
	snapshot := cloneEntries(ordered)
	b.mu.Unlock()

	s.mirror(ctx, sc.RunID, snapshot)
	s.broadcast(sc.RunID, snapshot)
}

// sortedLocked returns the run's entries ranked best-first. Caller holds b.mu.
// Tie-break: lower p99 wins, then submission id for determinism.
func (b *board) sortedLocked() []*models.LeaderboardEntry {
	out := make([]*models.LeaderboardEntry, 0, len(b.entries))
	for _, e := range b.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Composite != out[j].Composite {
			return out[i].Composite > out[j].Composite
		}
		if out[i].P99US != out[j].P99US {
			return out[i].P99US < out[j].P99US
		}
		return out[i].SubmissionID < out[j].SubmissionID
	})
	return out
}

// broadcast pushes a board snapshot to all WS subscribers of the run.
func (s *Service) broadcast(runID string, entries []models.LeaderboardEntry) {
	payload, err := json.Marshal(boardMessage{Type: "leaderboard", RunID: runID, Entries: entries})
	if err != nil {
		return
	}
	s.hub.Broadcast(runID, payload)
}

// mirror writes the board to Redis so REST reads survive restarts and other
// instances can hydrate. ZSET for ranking, per-entry JSON for detail, plus the
// run registry set.
func (s *Service) mirror(ctx context.Context, runID string, entries []models.LeaderboardEntry) {
	if s.redis == nil {
		return
	}
	pipe := s.redis.C.Pipeline()
	pipe.SAdd(ctx, store.RunsKey, runID)
	for i := range entries {
		e := entries[i]
		pipe.ZAdd(ctx, store.LeaderboardKey(runID), redis.Z{Score: e.Composite, Member: e.SubmissionID})
		if blob, err := json.Marshal(e); err == nil {
			pipe.Set(ctx, store.EntryKey(runID, e.SubmissionID), blob, 24*time.Hour)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		s.log.Warn("redis mirror failed", logpkg.F("err", err.Error()))
	}
}

// ---- read-model accessors used by the REST API ----

// Snapshot returns the ranked board for a run (optionally filtered).
func (s *Service) Snapshot(runID string, f Filter) []models.LeaderboardEntry {
	s.mu.RLock()
	b := s.runs[runID]
	s.mu.RUnlock()
	if b == nil {
		return nil
	}
	b.mu.RLock()
	ordered := b.sortedLocked()
	out := make([]models.LeaderboardEntry, 0, len(ordered))
	for i, e := range ordered {
		e.Rank = i + 1
		if f.keep(e) {
			out = append(out, *e)
		}
	}
	b.mu.RUnlock()
	return out
}

// Entry returns one contestant's current entry.
func (s *Service) Entry(runID, submissionID string) (models.LeaderboardEntry, bool) {
	s.mu.RLock()
	b := s.runs[runID]
	s.mu.RUnlock()
	if b == nil {
		return models.LeaderboardEntry{}, false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	e := b.entries[submissionID]
	if e == nil {
		return models.LeaderboardEntry{}, false
	}
	return *e, true
}

// Runs lists known runs with their contestant counts, newest activity first.
func (s *Service) Runs() []RunSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RunSummary, 0, len(s.order))
	for _, id := range s.order {
		b := s.runs[id]
		b.mu.RLock()
		n := len(b.entries)
		var latest time.Time
		for _, e := range b.entries {
			if e.UpdatedAt.After(latest) {
				latest = e.UpdatedAt
			}
		}
		b.mu.RUnlock()
		out = append(out, RunSummary{RunID: id, Contestants: n, LastUpdate: latest})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUpdate.After(out[j].LastUpdate) })
	return out
}

// Hydrate rebuilds in-memory state from Redis on startup so a restarted pod can
// serve the board immediately.
func (s *Service) Hydrate(ctx context.Context) {
	if s.redis == nil {
		return
	}
	runs, err := s.redis.C.SMembers(ctx, store.RunsKey).Result()
	if err != nil {
		return
	}
	for _, runID := range runs {
		subs, err := s.redis.C.ZRevRange(ctx, store.LeaderboardKey(runID), 0, -1).Result()
		if err != nil {
			continue
		}
		b := s.boardFor(runID)
		b.mu.Lock()
		for _, sub := range subs {
			blob, err := s.redis.C.Get(ctx, store.EntryKey(runID, sub)).Bytes()
			if err != nil {
				continue
			}
			var e models.LeaderboardEntry
			if json.Unmarshal(blob, &e) == nil {
				ee := e
				b.entries[sub] = &ee
			}
		}
		b.mu.Unlock()
	}
	s.log.Info("hydrated leaderboard from redis", logpkg.F("runs", len(runs)))
}

// boardMessage is the WS payload shape the frontend consumes.
type boardMessage struct {
	Type    string                    `json:"type"`
	RunID   string                    `json:"run_id"`
	Entries []models.LeaderboardEntry `json:"entries"`
}

// RunSummary is one row in the run selector.
type RunSummary struct {
	RunID       string    `json:"run_id"`
	Contestants int       `json:"contestants"`
	LastUpdate  time.Time `json:"last_update"`
}

func cloneEntries(in []*models.LeaderboardEntry) []models.LeaderboardEntry {
	out := make([]models.LeaderboardEntry, len(in))
	for i, e := range in {
		out[i] = *e
	}
	return out
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// ConsumeSandbox reads sandbox metric envelopes and updates board health.
func (s *Service) ConsumeSandbox(ctx context.Context, c *kafka.Consumer) error {
	for {
		msg, err := c.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.log.Error("sandbox read", logpkg.F("err", err.Error()))
			continue
		}
		env, err := events.UnmarshalEnvelope(msg.Value)
		if err != nil {
			_ = c.Commit(ctx, msg)
			continue
		}
		if env.Type == events.TypeSandboxMetrics {
			sm, err := env.AsSandboxMetrics()
			if err == nil && env.RunID != "" && env.SubmissionID != "" {
				s.ApplyHealth(ctx, env.RunID, env.SubmissionID, sm.Health)
			}
		}
		_ = c.Commit(ctx, msg)
	}
}

// ApplyHealth updates the Health field on an existing board entry. If the
// submission hasn't scored yet it has no entry and we silently skip; health
// will appear once the first score arrives (defaulting to "READY").
func (s *Service) ApplyHealth(ctx context.Context, runID, submissionID, health string) {
	if health == "" {
		return
	}
	b := s.boardFor(runID)
	b.mu.Lock()
	e := b.entries[submissionID]
	if e != nil && e.Health != health {
		s.log.Info("leaderboard health updated", logpkg.F(
			"run_id", runID, "submission_id", submissionID,
			"old_health", e.Health, "new_health", health))
		e.Health = health
		e.UpdatedAt = time.Now().UTC()
	}
	b.mu.Unlock()
}

