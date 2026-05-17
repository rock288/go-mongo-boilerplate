package health

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// MongoChecker pings the Mongo client. Critical: failure trips readiness.
type MongoChecker struct {
	client *mongo.Client
}

func NewMongoChecker(client *mongo.Client) *MongoChecker {
	return &MongoChecker{client: client}
}

func (m *MongoChecker) Name() string       { return "mongo" }
func (m *MongoChecker) Severity() Severity { return SeverityCritical }
func (m *MongoChecker) Check(ctx context.Context) error {
	return m.client.Ping(ctx, nil)
}

// KafkaChecker considers Kafka healthy when a poll/heartbeat happened within
// staleAfter. Degraded: failures degrade but do not block readiness.
type KafkaChecker struct {
	lastPollUnixNano atomic.Int64
	staleAfter       time.Duration
}

// NewKafkaChecker returns a checker bound to a staleAfter window. Worker
// process calls Heartbeat() after each PollFetches.
func NewKafkaChecker(staleAfter time.Duration) *KafkaChecker {
	if staleAfter <= 0 {
		staleAfter = 30 * time.Second
	}
	return &KafkaChecker{staleAfter: staleAfter}
}

func (k *KafkaChecker) Name() string       { return "kafka" }
func (k *KafkaChecker) Severity() Severity { return SeverityDegraded }

// Heartbeat records that a Kafka poll succeeded recently.
func (k *KafkaChecker) Heartbeat() {
	k.lastPollUnixNano.Store(time.Now().UnixNano())
}

func (k *KafkaChecker) Check(_ context.Context) error {
	last := k.lastPollUnixNano.Load()
	if last == 0 {
		return errors.New("kafka: no poll observed yet")
	}
	if time.Since(time.Unix(0, last)) > k.staleAfter {
		return errors.New("kafka: poll heartbeat stale")
	}
	return nil
}
