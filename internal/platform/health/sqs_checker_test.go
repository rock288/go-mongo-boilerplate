package health

import (
	"context"
	"testing"
	"time"
)

func TestSQSChecker_NameAndSeverity(t *testing.T) {
	c := NewSQSChecker(0)
	if c.Name() != "sqs" {
		t.Errorf("SQSChecker.Name() = %q", c.Name())
	}
	if c.Severity() != SeverityDegraded {
		t.Errorf("SQSChecker.Severity() = %v, want degraded", c.Severity())
	}
}

func TestSQSChecker_HeartbeatLifecycle(t *testing.T) {
	t.Parallel()

	c := NewSQSChecker(100 * time.Millisecond)
	if err := c.Check(context.Background()); err == nil {
		t.Fatal("fresh checker should report no-poll-yet error")
	}

	c.Heartbeat()
	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("post-heartbeat check failed: %v", err)
	}

	time.Sleep(250 * time.Millisecond)
	if err := c.Check(context.Background()); err == nil {
		t.Fatal("expected stale-poll error after staleAfter elapsed")
	}
}
