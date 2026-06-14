package kafka

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	kgo "github.com/segmentio/kafka-go"
)

// EnsureTopics creates each topic if it does not already exist. Idempotent:
// "topic already exists" is treated as success so every service can call this
// at boot without coordinating who creates what. Run topics get more partitions
// than analytics topics because raw order traffic is the highest-volume stream.
func EnsureTopics(ctx context.Context, brokers []string, partitions, replication int) error {
	if len(brokers) == 0 {
		return errors.New("no brokers configured")
	}
	conn, err := kgo.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	ctrlConn, err := kgo.DialContext(ctx, "tcp",
		net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer ctrlConn.Close()
	_ = ctrlConn.SetDeadline(time.Now().Add(15 * time.Second))

	// (topic, partitionsOverride). 0 => use the default `partitions`.
	specs := []struct {
		name string
		p    int
	}{
		{"telemetry.orders", partitions},
		{"telemetry.connections", partitions},
		{"telemetry.bot_metrics", partitions},
		{"telemetry.sandbox_metrics", maxInt(1, partitions/2)},
		{"analytics.window_aggregates", maxInt(1, partitions/2)},
		{"analytics.validation_results", maxInt(1, partitions/2)},
		{"analytics.scores", maxInt(1, partitions/4)},
		{"leaderboard.updates", maxInt(1, partitions/4)},
		{"telemetry.deadletter", 1},
	}

	configs := make([]kgo.TopicConfig, 0, len(specs))
	for _, s := range specs {
		configs = append(configs, kgo.TopicConfig{
			Topic:             s.name,
			NumPartitions:     s.p,
			ReplicationFactor: replication,
		})
	}
	if err := ctrlConn.CreateTopics(configs...); err != nil {
		// kafka-go returns nil when topics already exist; other errors bubble.
		return err
	}
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
