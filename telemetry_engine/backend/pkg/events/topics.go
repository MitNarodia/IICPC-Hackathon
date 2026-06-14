package events

// Redpanda/Kafka topic names. One topic per logical stream. Keeping order
// lifecycle events on a single topic (TopicOrders) preserves their relative
// order within a partition, which the validation engine depends on to replay
// the book deterministically.
const (
	// Raw ingest topics (produced by ingestion-service).
	TopicOrders         = "telemetry.orders"          // submitted/ack/filled/cancelled
	TopicConnections    = "telemetry.connections"     // opened/closed
	TopicBotMetrics     = "telemetry.bot_metrics"     // Track 2 AggregateView windows
	TopicSandboxMetrics = "telemetry.sandbox_metrics" // Track 1 resource samples

	// Derived/analytics topics (produced by the processing tier).
	TopicWindowAggregates = "analytics.window_aggregates" // stream-processor output
	TopicValidationResult = "analytics.validation_results" // validation-engine output
	TopicScores           = "analytics.scores"             // scoring-engine output

	// Fan-out topic the leaderboard-service tails to push live updates.
	TopicLeaderboardUpdates = "leaderboard.updates"

	// Dead-letter for events that fail decode/validation at ingestion.
	TopicDeadLetter = "telemetry.deadletter"
)

// Consumer group IDs. Each independent pipeline stage is its own group so they
// all receive the full stream (fan-out) while members WITHIN a group share the
// partitions (scale-out). See TRACK3_EXPLAINED.md §"Consumer groups".
const (
	GroupStreamProcessor = "track3.stream-processor"
	GroupValidation      = "track3.validation-engine"
	GroupScoring         = "track3.scoring-engine"
	GroupLeaderboard     = "track3.leaderboard-service"
)

// TopicForType returns the raw ingest topic an event of the given type belongs
// on. Used by the ingestion service to route a decoded envelope.
func TopicForType(t EventType) string {
	switch t {
	case TypeOrderSubmitted, TypeOrderAck, TypeOrderFilled, TypeOrderCancelled:
		return TopicOrders
	case TypeConnectionOpened, TypeConnectionClosed:
		return TopicConnections
	case TypeBotMetrics:
		return TopicBotMetrics
	case TypeSandboxMetrics:
		return TopicSandboxMetrics
	default:
		return TopicDeadLetter
	}
}

// AllRawTopics lists every topic the ingestion service may write, used by the
// admin bootstrap to create topics with the right partition counts.
func AllRawTopics() []string {
	return []string{
		TopicOrders, TopicConnections, TopicBotMetrics, TopicSandboxMetrics,
		TopicWindowAggregates, TopicValidationResult, TopicScores,
		TopicLeaderboardUpdates, TopicDeadLetter,
	}
}
