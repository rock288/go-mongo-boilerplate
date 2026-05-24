package health

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

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
