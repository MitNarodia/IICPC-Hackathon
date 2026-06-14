// Package kafka wraps segmentio/kafka-go with the few patterns Track 3 needs:
// a keyed producer (so a run's events stay on one partition) and a consumer
// group reader with manual commit. Works against Redpanda unchanged because
// Redpanda speaks the Kafka protocol.
package kafka

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/iicpc/track3/telemetry-engine/pkg/events"
	kgo "github.com/segmentio/kafka-go"
)

// Producer publishes envelopes. It batches internally and is safe for
// concurrent use by multiple goroutines.
type Producer struct {
	w *kgo.Writer
}

// NewProducer builds a producer. Balancer is Hash(key) so identical keys (a
// run's PartitionKey) always land on the same partition — the ordering
// guarantee the validation engine relies on.
func NewProducer(brokers []string, useTLS bool) *Producer {
	t := &kgo.Transport{}
	if useTLS {
		t.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return &Producer{
		w: &kgo.Writer{
			Addr:         kgo.TCP(brokers...),
			Balancer:     &kgo.Hash{},
			RequiredAcks: kgo.RequireAll, // durability over raw throughput
			BatchTimeout: 5 * time.Millisecond,
			Compression:  kgo.Snappy,
			Transport:    t,
			Async:        false,
		},
	}
}

// Publish marshals and sends an envelope to its routed topic, keyed by run so
// ordering is preserved per (run, submission).
func (p *Producer) Publish(ctx context.Context, topic string, e *events.Envelope) error {
	val, err := e.Marshal()
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return p.w.WriteMessages(ctx, kgo.Message{
		Topic: topic,
		Key:   []byte(e.PartitionKey()),
		Value: val,
	})
}

// PublishRaw sends a pre-serialized value with an explicit key. Used by derived
// stages (window aggregates, scores) whose payloads aren't Envelopes.
func (p *Producer) PublishRaw(ctx context.Context, topic, key string, value []byte) error {
	return p.w.WriteMessages(ctx, kgo.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: value,
	})
}

func (p *Producer) Close() error { return p.w.Close() }

// Consumer is a consumer-group reader over one topic. Each call to Read blocks
// for the next message; the caller commits explicitly after processing so a
// crash mid-process re-delivers rather than silently dropping.
type Consumer struct {
	r *kgo.Reader
}

// NewConsumer joins `group` on `topic`. Members of the same group share the
// topic's partitions; different groups each get the full stream (fan-out).
func NewConsumer(brokers []string, group, topic string, useTLS bool) *Consumer {
	var dialer *kgo.Dialer
	if useTLS {
		dialer = &kgo.Dialer{Timeout: 10 * time.Second, TLS: &tls.Config{MinVersion: tls.VersionTLS12}}
	}
	return &Consumer{
		r: kgo.NewReader(kgo.ReaderConfig{
			Brokers:        brokers,
			GroupID:        group,
			Topic:          topic,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: 0, // 0 => manual commit via CommitMessages
			StartOffset:    kgo.FirstOffset,
			Dialer:         dialer,
		}),
	}
}

// Message is a thin alias so callers don't import kafka-go directly.
type Message = kgo.Message

// Read returns the next message in the group. Blocks until one is available or
// ctx is cancelled.
func (c *Consumer) Read(ctx context.Context) (Message, error) {
	return c.r.FetchMessage(ctx)
}

// Commit acknowledges a message has been fully processed.
func (c *Consumer) Commit(ctx context.Context, msgs ...Message) error {
	return c.r.CommitMessages(ctx, msgs...)
}

func (c *Consumer) Close() error { return c.r.Close() }
