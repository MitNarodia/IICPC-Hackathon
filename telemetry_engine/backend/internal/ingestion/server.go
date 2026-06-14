// Package ingestion is Track 3's front door. It accepts telemetry from Track 2
// (the C++ bot fleet) and Track 1 (the sandbox runtime), validates it at the
// boundary, stamps ingest time, and routes each event to the correct Redpanda
// topic. Everything downstream trusts that whatever made it past this service
// is well-formed — so this is where we are strict (defense at the boundary).
//
// THREE INGRESS SHAPES:
//   - POST /v1/events     : a single Envelope or a JSON array of Envelopes.
//   - GET  /v1/ws         : a WebSocket carrying a stream of Envelopes (the
//                           high-throughput path the bot fleet uses).
//   - POST /v1/track2/...  and /v1/track1/... : thin shims that accept Track 2's
//                           AggregateView and Track 1's sandbox sample in their
//                           native shape and wrap them into Envelopes, so the
//                           other tracks need almost no Track-3-specific code.
package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	"github.com/iicpc/track3/telemetry-engine/pkg/kafka"
	logpkg "github.com/iicpc/track3/telemetry-engine/pkg/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Server holds the ingestion dependencies.
type Server struct {
	producer *kafka.Producer
	log      *logpkg.Logger
	upgrader websocket.Upgrader

	ingested   *prometheus.CounterVec
	deadletter prometheus.Counter
	bytesIn    prometheus.Counter
}

// NewServer builds the ingestion HTTP server.
func NewServer(producer *kafka.Producer, log *logpkg.Logger) *Server {
	return &Server{
		producer: producer,
		log:      log,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1 << 16,
			WriteBufferSize: 1 << 12,
			// Bots connect from inside the cluster; origin checks are handled by
			// network policy, so we accept any origin here.
			CheckOrigin: func(*http.Request) bool { return true },
		},
		ingested: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "track3_ingest_events_total",
			Help: "Telemetry events accepted and routed, by type.",
		}, []string{"type"}),
		deadletter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "track3_ingest_deadletter_total",
			Help: "Events rejected at the boundary and dead-lettered.",
		}),
		bytesIn: promauto.NewCounter(prometheus.CounterOpts{
			Name: "track3_ingest_bytes_total",
			Help: "Raw bytes accepted at ingest.",
		}),
	}
}

// Routes registers the ingestion endpoints on a mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/events", s.handleEvents)
	mux.HandleFunc("/v1/ws", s.handleWS)
	mux.HandleFunc("/v1/track2/bot-metrics", s.handleTrack2BotMetrics)
	mux.HandleFunc("/v1/track1/sandbox", s.handleTrack1Sandbox)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
}

// handleEvents accepts one Envelope or an array of them. Limits the body to
// guard against memory-exhaustion (a malicious or buggy producer).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20)) // 16 MiB cap
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	s.bytesIn.Add(float64(len(body)))

	envs, err := decodeEnvelopes(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	accepted, rejected := 0, 0
	for _, e := range envs {
		if s.route(r.Context(), e) {
			accepted++
		} else {
			rejected++
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": accepted, "rejected": rejected})
}

// handleWS upgrades to a WebSocket and ingests a continuous stream of Envelopes
// (one JSON object per message). This is the bot fleet's primary path.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Warn("ws upgrade failed", logpkg.F("err", err.Error()))
		return
	}
	defer conn.Close()
	conn.SetReadLimit(8 << 20)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return // client closed or error; just end the session
		}
		s.bytesIn.Add(float64(len(msg)))
		e, derr := events.UnmarshalEnvelope(msg)
		if derr != nil {
			s.deadLetter(r.Context(), msg, derr.Error())
			continue
		}
		s.route(r.Context(), e)
	}
}

// route validates, stamps, and produces one envelope. Returns false if the
// event was dead-lettered.
func (s *Server) route(ctx context.Context, e *events.Envelope) bool {
	if err := e.Validate(); err != nil {
		raw, _ := e.Marshal()
		s.deadLetter(ctx, raw, err.Error())
		return false
	}
	e.IngestTS = time.Now().UnixNano()
	topic := events.TopicForType(e.Type)
	if err := s.producer.Publish(ctx, topic, e); err != nil {
		s.log.Error("produce failed", logpkg.F("topic", topic, "err", err.Error()))
		raw, _ := e.Marshal()
		s.deadLetter(ctx, raw, "produce: "+err.Error())
		return false
	}
	s.ingested.WithLabelValues(string(e.Type)).Inc()
	return true
}

func (s *Server) deadLetter(ctx context.Context, raw []byte, reason string) {
	s.deadletter.Inc()
	_ = s.producer.PublishRaw(ctx, events.TopicDeadLetter, reason, raw)
	s.log.Warn("dead-lettered event", logpkg.F("reason", reason))
}

func decodeEnvelopes(body []byte) ([]*events.Envelope, error) {
	body = trimLeadingSpace(body)
	if len(body) == 0 {
		return nil, errors.New("empty body")
	}
	if body[0] == '[' {
		var arr []*events.Envelope
		if err := json.Unmarshal(body, &arr); err != nil {
			return nil, errors.New("invalid JSON array of envelopes")
		}
		return arr, nil
	}
	var e events.Envelope
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, errors.New("invalid envelope JSON")
	}
	return []*events.Envelope{&e}, nil
}

func trimLeadingSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	return b[i:]
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
