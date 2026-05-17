package kafka

import (
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// NewProducer creates a franz-go client tuned for durable, batched writes.
func NewProducer(cfg config.KafkaConfig) (*kgo.Client, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()),
	}
	return kgo.NewClient(opts...)
}

// NewConsumer creates a franz-go client bound to a consumer group with
// auto-commit disabled (caller commits after successful handle).
func NewConsumer(cfg config.KafkaConfig) (*kgo.Client, error) {
	return NewConsumerForTopics(cfg, cfg.GroupID, cfg.Topic)
}

// NewConsumerForTopics is the explicit form used by the retry consumer.
func NewConsumerForTopics(cfg config.KafkaConfig, groupID string, topics ...string) (*kgo.Client, error) {
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topics...),
		kgo.DisableAutoCommit(),
	}
	return kgo.NewClient(opts...)
}

// CleanupClient returns a cleanup func that closes the client.
func CleanupClient(client *kgo.Client) func() {
	return func() {
		client.Close()
	}
}
