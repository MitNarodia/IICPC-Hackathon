package telemetry

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type Logger struct {
	mu      sync.Mutex
	out     io.Writer
	service string
}

type Field map[string]interface{}

func NewLogger(service string, out io.Writer) *Logger {
	if out == nil {
		out = os.Stdout
	}
	return &Logger{out: out, service: service}
}

func (l *Logger) Info(message string, fields Field) {
	l.write("info", message, fields)
}

func (l *Logger) Error(message string, fields Field) {
	l.write("error", message, fields)
}

func (l *Logger) write(level, message string, fields Field) {
	if fields == nil {
		fields = Field{}
	}
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["level"] = level
	fields["service"] = l.service
	fields["message"] = message
	payload, _ := json.Marshal(fields)
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(payload, '\n'))
}
