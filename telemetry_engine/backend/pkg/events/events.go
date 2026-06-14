// Package events defines the canonical Track 3 event contract: the Envelope,
// every telemetry payload type, and the Redpanda topic names. Every service in
// the pipeline imports this package and nothing else for its wire types, so
// there is exactly one source of truth for "what an event looks like".
//
// Serialization: events travel as JSON on Redpanda. JSON was chosen over raw
// Protobuf on the bus for debuggability (you can `rpk topic consume` and read
// it) and because Track 2 already emits JSON, minimizing translation. The
// Protobuf schema in api/proto/telemetry.proto documents the same shape and is
// available as a drop-in for teams that need the wire-size win.
package events

import "encoding/json"

// EventType is the discriminator stored in Envelope.Type so a consumer can
// decode the right payload without trial-and-error unmarshalling.
type EventType string

const (
	TypeOrderSubmitted   EventType = "order_submitted"
	TypeOrderAck         EventType = "order_ack"
	TypeOrderFilled      EventType = "order_filled"
	TypeOrderCancelled   EventType = "order_cancelled"
	TypeConnectionOpened EventType = "connection_opened"
	TypeConnectionClosed EventType = "connection_closed"
	TypeBotMetrics       EventType = "bot_metrics"
	TypeSandboxMetrics   EventType = "sandbox_metrics"
)

// Side mirrors Track 2's market::Side.
type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

// OrderKind mirrors Track 2's market::OrderType.
type OrderKind string

const (
	KindLimit  OrderKind = "limit"
	KindMarket OrderKind = "market"
	KindCancel OrderKind = "cancel"
)

// Envelope wraps every event on the bus. RunID + SubmissionID together scope a
// benchmark; Seq lets a consumer detect gaps within a (Source) partition.
//
// Field meanings:
//   - RunID:        which benchmark run (one "Run" button press) this belongs to.
//   - SubmissionID: the contestant matching-engine under test (Track 1 UUIDv7).
//   - Seq:          monotonic per-(run, source) sequence; gap detection / ordering.
//   - EmitTS:       emitter capture time (unix nanoseconds).
//   - IngestTS:     ingestion receive time (unix ns), stamped by ingestion-svc.
//   - Source:       emitter identity, e.g. "bot-fleet/shard-3".
//   - Type:         payload discriminator (see EventType).
//   - Payload:      raw JSON of exactly one of the payload structs below.
type Envelope struct {
	RunID        string          `json:"run_id"`
	SubmissionID string          `json:"submission_id"`
	Seq          uint64          `json:"seq"`
	EmitTS       int64           `json:"emit_ts"`
	IngestTS     int64           `json:"ingest_ts,omitempty"`
	Source       string          `json:"source"`
	Type         EventType       `json:"type"`
	Payload      json.RawMessage `json:"payload"`
}

// PartitionKey returns the Kafka key used to keep all events for one benchmark
// run co-located on the same partition. Co-location guarantees that a single
// validation-engine consumer sees a run's full, ordered order stream — which
// is required to deterministically reconstruct the order book.
func (e *Envelope) PartitionKey() string {
	return e.RunID + ":" + e.SubmissionID
}

// OrderSubmitted: a bot pushed an order onto the wire (Track 2 stamps SendTS
// just before the socket write). The latency clock starts here.
type OrderSubmitted struct {
	BotID         uint64    `json:"bot_id"`
	OrderID       uint64    `json:"order_id"`
	Side          Side      `json:"side"`
	Kind          OrderKind `json:"kind"`
	Price         float64   `json:"price"`            // ignored for market/cancel
	Quantity      uint32    `json:"quantity"`
	SendTS        int64     `json:"send_ts"`          // unix ns; latency basis
	TargetOrderID uint64    `json:"target_order_id"`  // cancel target, else 0
}

// OrderAck: the engine accepted/rejected the order. Closes the latency loop.
type OrderAck struct {
	BotID        uint64 `json:"bot_id"`
	OrderID      uint64 `json:"order_id"`
	Accepted     bool   `json:"accepted"`
	RejectReason string `json:"reject_reason,omitempty"`
	RecvTS       int64  `json:"recv_ts"`        // unix ns
	AckLatencyUS int64  `json:"ack_latency_us"` // emitter-measured round trip
}

// OrderFilled: a (partial/full) fill reported by the engine. The validation
// engine uses MakerOrderID + FillPrice + FillQuantity to check matching.
type OrderFilled struct {
	BotID             uint64  `json:"bot_id"`
	OrderID           uint64  `json:"order_id"`        // aggressor
	MakerOrderID      uint64  `json:"maker_order_id"`  // resting order hit
	FillPrice         float64 `json:"fill_price"`
	FillQuantity      uint32  `json:"fill_quantity"`
	RemainingQuantity uint32  `json:"remaining_quantity"`
	TradeID           uint64  `json:"trade_id"`
	RecvTS            int64   `json:"recv_ts"`
}

// OrderCancelled: a resting order was removed from the book.
type OrderCancelled struct {
	BotID             uint64 `json:"bot_id"`
	OrderID           uint64 `json:"order_id"`
	CancelledQuantity uint32 `json:"cancelled_quantity"`
	Reason            string `json:"reason"`
	RecvTS            int64  `json:"recv_ts"`
}

// ConnectionOpened: a WS session came up between a bot and the engine.
type ConnectionOpened struct {
	ConnectionID uint64 `json:"connection_id"`
	BotID        uint64 `json:"bot_id"`
	RemoteAddr   string `json:"remote_addr"`
	OpenedTS     int64  `json:"opened_ts"`
	HandshakeUS  int64  `json:"handshake_us"`
}

// ConnectionClosed: a WS session ended (graceful or error).
type ConnectionClosed struct {
	ConnectionID uint64 `json:"connection_id"`
	BotID        uint64 `json:"bot_id"`
	CloseReason  string `json:"close_reason"`
	ClosedTS     int64  `json:"closed_ts"`
	BytesSent    uint64 `json:"bytes_sent"`
	BytesRecv    uint64 `json:"bytes_recv"`
}

// BotMetrics: a pre-aggregated window straight from Track 2's AggregateView.
// This is the fast path — Track 3 can trust these percentiles (they were
// computed from a full HDR histogram) or re-derive its own from raw events.
type BotMetrics struct {
	ShardID       uint32  `json:"shard_id"`
	Transactions  uint64  `json:"transactions"`
	Errors        uint64  `json:"errors"`
	Timeouts      uint64  `json:"timeouts"`
	WindowSeconds float64 `json:"window_seconds"`
	P50US         int64   `json:"p50_us"`
	P90US         int64   `json:"p90_us"`
	P99US         int64   `json:"p99_us"`
	MeanUS        float64 `json:"mean_us"`
	// HdrBuckets carries the raw histogram (base64) so the stream processor can
	// merge shards EXACTLY rather than averaging pre-computed percentiles.
	HdrBuckets    []byte `json:"hdr_buckets,omitempty"`
	WindowStartTS int64  `json:"window_start_ts"`
	WindowEndTS   int64  `json:"window_end_ts"`
}

// SandboxMetrics: Track 1 resource/health sample for the contestant Pod.
type SandboxMetrics struct {
	PodName            string  `json:"pod_name"`
	Namespace          string  `json:"namespace"`
	CPUMillicores      float64 `json:"cpu_millicores"`
	CPULimitMillicores float64 `json:"cpu_limit_millicores"`
	MemoryBytes        uint64  `json:"memory_bytes"`
	MemoryLimitBytes   uint64  `json:"memory_limit_bytes"`
	OpenFDs            uint32  `json:"open_fds"`
	ActiveConnections  uint32  `json:"active_connections"`
	Health             string  `json:"health"`
	OOMKilled          bool    `json:"oom_killed"`
	RestartCount       uint32  `json:"restart_count"`
	SampleTS           int64   `json:"sample_ts"`
}
