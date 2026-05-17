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

// SQSChecker mirrors KafkaChecker for the SQS consumer. Heartbeat is called
// after each successful ReceiveMessage (even when the response is empty —
// an empty poll still proves the consumer is healthy). Severity: Degraded.
type SQSChecker struct {
	lastPollUnixNano atomic.Int64
	staleAfter       time.Duration
}

// NewSQSChecker returns an SQS heartbeat checker bound to staleAfter.
func NewSQSChecker(staleAfter time.Duration) *SQSChecker {
	if staleAfter <= 0 {
		staleAfter = 30 * time.Second
	}
	return &SQSChecker{staleAfter: staleAfter}
}

func (s *SQSChecker) Name() string       { return "sqs" }
func (s *SQSChecker) Severity() Severity { return SeverityDegraded }

// Heartbeat records that an SQS poll succeeded recently.
func (s *SQSChecker) Heartbeat() {
	s.lastPollUnixNano.Store(time.Now().UnixNano())
}

func (s *SQSChecker) Check(_ context.Context) error {
	last := s.lastPollUnixNano.Load()
	if last == 0 {
		return errors.New("sqs: no poll observed yet")
	}
	if time.Since(time.Unix(0, last)) > s.staleAfter {
		return errors.New("sqs: poll heartbeat stale")
	}
	return nil
}
