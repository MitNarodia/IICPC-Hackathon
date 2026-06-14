// Package kafka provides a Redpanda/Kafka-backed implementation of events.Publisher
// and a consumer runtime for event-driven services.
package kafka

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"time"

	kgo "github.com/segmentio/kafka-go"

	"github.com/iicpc/track1/submission-engine/pkg/events"
)

// Producer implements events.Publisher by writing envelopes to Redpanda/Kafka.
// Messages are keyed by SubmissionID for partition ordering.
type Producer struct {
	writer *kgo.Writer
}

// NewProducer creates a Kafka writer (auto-topic-creation compatible with Redpanda).
func NewProducer(brokers []string) *Producer {
	w := &kgo.Writer{
		Addr:         kgo.TCP(brokers...),
		Balancer:     &kgo.LeastBytes{},
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kgo.RequireAll,
	}
	return &Producer{writer: w}
}

// Publish serializes the envelope and writes it to the topic derived from the event type.
func (p *Producer) Publish(ctx context.Context, env events.Envelope) error {
	value, err := events.Marshal(env)
	if err != nil {
		return fmt.Errorf("kafka: marshal: %w", err)
	}
	msg := kgo.Message{
		Topic: env.Topic(),
		Key:   []byte(env.Key()),
		Value: value,
	}
	return p.writer.WriteMessages(ctx, msg)
}

// Close closes the writer.
func (p *Producer) Close() error { return p.writer.Close() }

// --- Consumer ---

// HandlerFunc is called for each successfully decoded envelope.
type HandlerFunc func(ctx context.Context, env events.Envelope) error

// Consumer reads from one or more topics using a consumer group, deserializes
// envelopes, and dispatches to registered handlers by event type.
type Consumer struct {
	reader   *kgo.Reader
	handlers map[events.Type]HandlerFunc
}

// ConsumerConfig configures the consumer group.
type ConsumerConfig struct {
	Brokers []string
	GroupID string
	Topics  []string
}

// NewConsumer creates a consumer group reader.
func NewConsumer(cfg ConsumerConfig) *Consumer {
	r := kgo.NewReader(kgo.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		GroupTopics:    cfg.Topics,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    kgo.LastOffset,
	})
	return &Consumer{
		reader:   r,
		handlers: make(map[events.Type]HandlerFunc),
	}
}

// Handle registers a handler for a specific event type.
func (c *Consumer) Handle(eventType events.Type, fn HandlerFunc) {
	c.handlers[eventType] = fn
}

// Run starts the consume loop. It blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("kafka consumer: fetch error: %v", err)
			time.Sleep(time.Second)
			continue
		}
		env, err := events.Unmarshal(msg.Value)
		if err != nil {
			log.Printf("kafka consumer: unmarshal error (topic=%s offset=%d): %v", msg.Topic, msg.Offset, err)
			_ = c.reader.CommitMessages(ctx, msg) // skip poison pill
			continue
		}
		handler, ok := c.handlers[env.Type]
		if !ok {
			// No handler for this type — commit and move on.
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}
		if err := handler(ctx, env); err != nil {
			log.Printf("kafka consumer: handler error (type=%s sub=%s): %v", env.Type, env.SubmissionID, err)
			// Commit anyway to avoid blocking the partition; dead-letter in production.
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}
		_ = c.reader.CommitMessages(ctx, msg)
	}
}

// Close closes the reader.
func (c *Consumer) Close() error { return c.reader.Close() }

// EnsureTopics creates topics if they don't exist (Redpanda auto-create is usually on,
// but this makes it explicit for partitioning).
func EnsureTopics(ctx context.Context, brokers []string, topics []string, partitions int) error {
	conn, err := kgo.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("kafka: dial: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("kafka: controller: %w", err)
	}
	controllerAddr := net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port))
	controllerConn, err := kgo.DialContext(ctx, "tcp", controllerAddr)
	if err != nil {
		return fmt.Errorf("kafka: dial controller: %w", err)
	}
	defer controllerConn.Close()

	topicConfigs := make([]kgo.TopicConfig, len(topics))
	for i, t := range topics {
		topicConfigs[i] = kgo.TopicConfig{
			Topic:             t,
			NumPartitions:     partitions,
			ReplicationFactor: 1,
		}
	}
	return controllerConn.CreateTopics(topicConfigs...)
}
