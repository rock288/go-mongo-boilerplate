package health

import "testing"

func TestKafkaChecker_NameAndSeverity(t *testing.T) {
	c := NewKafkaChecker(0) // 0 → defaults to 30s
	if c.Name() != "kafka" {
		t.Errorf("KafkaChecker.Name() = %q", c.Name())
	}
	if c.Severity() != SeverityDegraded {
		t.Errorf("KafkaChecker.Severity() = %v, want degraded", c.Severity())
	}
}
