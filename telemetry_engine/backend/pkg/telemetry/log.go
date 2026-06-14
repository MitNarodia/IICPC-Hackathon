// Package log provides a tiny structured-JSON logger shared by every service.
// One line per event, always machine-parseable, always carrying run/submission
// context when available. We deliberately avoid a heavyweight logging framework
// to keep the binaries small and the hot path allocation-light.
package log

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

var levelRank = map[Level]int{LevelDebug: 0, LevelInfo: 1, LevelWarn: 2, LevelError: 3}

// Logger writes structured JSON lines to stderr. Safe for concurrent use.
type Logger struct {
	mu      sync.Mutex
	service string
	min     int
	fields  map[string]any
}

// New returns a Logger tagged with the service name. The minimum level is read
// from the LOG_LEVEL env var (default "info").
func New(service string) *Logger {
	min := levelRank[LevelInfo]
	if lvl, ok := levelRank[Level(os.Getenv("LOG_LEVEL"))]; ok {
		min = lvl
	}
	return &Logger{service: service, min: min, fields: map[string]any{}}
}

// With returns a child logger that always includes the given fields. Used to
// pin run_id/submission_id onto every line within a processing scope.
func (l *Logger) With(kv map[string]any) *Logger {
	merged := make(map[string]any, len(l.fields)+len(kv))
	for k, v := range l.fields {
		merged[k] = v
	}
	for k, v := range kv {
		merged[k] = v
	}
	return &Logger{service: l.service, min: l.min, fields: merged}
}

func (l *Logger) log(level Level, msg string, kv map[string]any) {
	if levelRank[level] < l.min {
		return
	}
	rec := make(map[string]any, len(l.fields)+len(kv)+4)
	rec["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	rec["level"] = string(level)
	rec["service"] = l.service
	rec["msg"] = msg
	for k, v := range l.fields {
		rec[k] = v
	}
	for k, v := range kv {
		rec[k] = v
	}
	b, err := json.Marshal(rec)
	if err != nil {
		b = []byte(`{"level":"error","msg":"log marshal failed"}`)
	}
	l.mu.Lock()
	os.Stderr.Write(append(b, '\n'))
	l.mu.Unlock()
}

func (l *Logger) Debug(msg string, kv ...map[string]any) { l.log(LevelDebug, msg, merge(kv)) }
func (l *Logger) Info(msg string, kv ...map[string]any)  { l.log(LevelInfo, msg, merge(kv)) }
func (l *Logger) Warn(msg string, kv ...map[string]any)  { l.log(LevelWarn, msg, merge(kv)) }
func (l *Logger) Error(msg string, kv ...map[string]any) { l.log(LevelError, msg, merge(kv)) }

func merge(kv []map[string]any) map[string]any {
	if len(kv) == 0 {
		return nil
	}
	if len(kv) == 1 {
		return kv[0]
	}
	out := map[string]any{}
	for _, m := range kv {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// F is a convenience shorthand for a single-call field map.
func F(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	return m
}
