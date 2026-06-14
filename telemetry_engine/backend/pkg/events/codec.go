package events

import (
	"encoding/json"
	"fmt"
	"time"
)

// NewEnvelope builds an Envelope around a typed payload, marshalling the
// payload to JSON and stamping the emit time if the caller passed zero.
func NewEnvelope(runID, submissionID, source string, seq uint64, t EventType, payload any) (*Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	return &Envelope{
		RunID:        runID,
		SubmissionID: submissionID,
		Seq:          seq,
		EmitTS:       time.Now().UnixNano(),
		Source:       source,
		Type:         t,
		Payload:      raw,
	}, nil
}

// Marshal serializes an Envelope for the wire.
func (e *Envelope) Marshal() ([]byte, error) { return json.Marshal(e) }

// UnmarshalEnvelope parses a raw Kafka value into an Envelope (payload stays
// raw until the consumer decodes the specific type it cares about).
func UnmarshalEnvelope(b []byte) (*Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return &e, nil
}

// decode is the generic payload decoder used by the typed helpers below.
func decode[T any](e *Envelope) (T, error) {
	var v T
	if err := json.Unmarshal(e.Payload, &v); err != nil {
		return v, fmt.Errorf("decode %s: %w", e.Type, err)
	}
	return v, nil
}

func (e *Envelope) AsOrderSubmitted() (OrderSubmitted, error)     { return decode[OrderSubmitted](e) }
func (e *Envelope) AsOrderAck() (OrderAck, error)                 { return decode[OrderAck](e) }
func (e *Envelope) AsOrderFilled() (OrderFilled, error)           { return decode[OrderFilled](e) }
func (e *Envelope) AsOrderCancelled() (OrderCancelled, error)     { return decode[OrderCancelled](e) }
func (e *Envelope) AsConnectionOpened() (ConnectionOpened, error) { return decode[ConnectionOpened](e) }
func (e *Envelope) AsConnectionClosed() (ConnectionClosed, error) { return decode[ConnectionClosed](e) }
func (e *Envelope) AsBotMetrics() (BotMetrics, error)             { return decode[BotMetrics](e) }
func (e *Envelope) AsSandboxMetrics() (SandboxMetrics, error)     { return decode[SandboxMetrics](e) }

// Validate performs cheap structural checks at the ingestion boundary. Anything
// that fails here is routed to the dead-letter topic instead of poisoning the
// pipeline. (Defense at the system boundary — see security requirements.)
func (e *Envelope) Validate() error {
	if e.RunID == "" {
		return fmt.Errorf("missing run_id")
	}
	if e.SubmissionID == "" {
		return fmt.Errorf("missing submission_id")
	}
	if e.Type == "" {
		return fmt.Errorf("missing type")
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("empty payload")
	}
	switch e.Type {
	case TypeOrderSubmitted, TypeOrderAck, TypeOrderFilled, TypeOrderCancelled,
		TypeConnectionOpened, TypeConnectionClosed, TypeBotMetrics, TypeSandboxMetrics:
		return nil
	default:
		return fmt.Errorf("unknown event type %q", e.Type)
	}
}
