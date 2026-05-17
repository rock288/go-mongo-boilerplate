package health

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type stubChecker struct {
	name string
	sev  Severity
	err  error
}

func (s *stubChecker) Name() string                  { return s.name }
func (s *stubChecker) Severity() Severity            { return s.sev }
func (s *stubChecker) Check(_ context.Context) error { return s.err }

func TestReadiness_NoCheckersIsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRegistry()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/readyz", nil)
	r.Readiness(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestReadiness_BelowFailureThresholdStaysReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chk := &stubChecker{name: "mongo", sev: SeverityCritical, err: errors.New("boom")}
	r := NewRegistry(WithFailureLimit(3))
	r.Register(chk)
	for i := 0; i < 2; i++ {
		r.runAll(context.Background())
	}
	if !r.ready() {
		t.Fatal("expected ready while below failure threshold")
	}
}

func TestReadiness_TripsAfterThreshold(t *testing.T) {
	chk := &stubChecker{name: "mongo", sev: SeverityCritical, err: errors.New("boom")}
	r := NewRegistry(WithFailureLimit(3))
	r.Register(chk)
	for i := 0; i < 3; i++ {
		r.runAll(context.Background())
	}
	if r.ready() {
		t.Fatal("expected not_ready after threshold")
	}
}

func TestReadiness_DegradedDoesNotTrip(t *testing.T) {
	chk := &stubChecker{name: "kafka", sev: SeverityDegraded, err: errors.New("boom")}
	r := NewRegistry(WithFailureLimit(1))
	r.Register(chk)
	r.runAll(context.Background())
	if !r.ready() {
		t.Fatal("degraded failure must not flip readiness")
	}
}

func TestReadiness_RecoveryResetsCounter(t *testing.T) {
	chk := &stubChecker{name: "mongo", sev: SeverityCritical, err: errors.New("boom")}
	r := NewRegistry(WithFailureLimit(2))
	r.Register(chk)
	r.runAll(context.Background())
	r.runAll(context.Background())
	if r.ready() {
		t.Fatal("expected unready")
	}
	chk.err = nil
	r.runAll(context.Background())
	if !r.ready() {
		t.Fatal("expected recovery → ready")
	}
}

func TestCheckers_NameAndSeverityGetters(t *testing.T) {
	mongo := NewMongoChecker(nil)
	if mongo.Name() != "mongo" {
		t.Errorf("MongoChecker.Name() = %q", mongo.Name())
	}
	if mongo.Severity() != SeverityCritical {
		t.Errorf("MongoChecker.Severity() = %v, want critical", mongo.Severity())
	}

	kafka := NewKafkaChecker(0) // 0 → defaults to 30s
	if kafka.Name() != "kafka" {
		t.Errorf("KafkaChecker.Name() = %q", kafka.Name())
	}
	if kafka.Severity() != SeverityDegraded {
		t.Errorf("KafkaChecker.Severity() = %v, want degraded", kafka.Severity())
	}

	sqs := NewSQSChecker(0)
	if sqs.Name() != "sqs" {
		t.Errorf("SQSChecker.Name() = %q", sqs.Name())
	}
	if sqs.Severity() != SeverityDegraded {
		t.Errorf("SQSChecker.Severity() = %v, want degraded", sqs.Severity())
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

func TestWithCheckTimeout_AppliesOption(t *testing.T) {
	r := NewRegistry(WithCheckTimeout(123 * time.Millisecond))
	if r.timeout != 123*time.Millisecond {
		t.Errorf("WithCheckTimeout: got %v", r.timeout)
	}
}

func TestLiveness_AlwaysReturns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRegistry()
	// Add a critical checker that's failing — liveness must still 200
	chk := &stubChecker{name: "x", sev: SeverityCritical, err: errors.New("boom")}
	r.Register(chk)
	for i := 0; i < 10; i++ {
		r.runAll(context.Background())
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/healthz", nil)
	r.Liveness(c)
	if w.Code != 200 {
		t.Fatalf("expected liveness 200, got %d", w.Code)
	}
}

func TestDetailedReadiness_ExposesPerCheckBreakdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRegistry()
	r.Register(&stubChecker{name: "mongo", sev: SeverityCritical})
	r.Register(&stubChecker{name: "kafka", sev: SeverityDegraded, err: errors.New("lag")})
	r.runAll(context.Background())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/internal/readyz", nil)
	r.DetailedReadiness(c)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "mongo") || !strings.Contains(body, "kafka") {
		t.Fatalf("expected per-check detail, got %s", body)
	}
	if !strings.Contains(body, "critical") || !strings.Contains(body, "degraded") {
		t.Fatalf("expected severity names, got %s", body)
	}
}

func TestDetailedReadiness_ReturnsServiceUnavailableWhenCriticalFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRegistry(WithFailureLimit(1))
	r.Register(&stubChecker{name: "mongo", sev: SeverityCritical, err: errors.New("down")})
	r.runAll(context.Background())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/internal/readyz", nil)
	r.DetailedReadiness(c)
	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not_ready") {
		t.Fatalf("expected not_ready status, got %s", w.Body.String())
	}
}

func TestSnapshot_ReturnsCopy(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubChecker{name: "x", sev: SeverityCritical})
	r.runAll(context.Background())
	snap1 := r.Snapshot()
	if len(snap1) != 1 {
		t.Fatalf("expected 1 result, got %d", len(snap1))
	}
	// Mutate snapshot — must not affect registry's internal map
	snap1[0].Name = "MUTATED"
	snap2 := r.Snapshot()
	if snap2[0].Name == "MUTATED" {
		t.Fatal("Snapshot must return copies, not aliases")
	}
}

func TestSeverityName_HandlesUnknown(t *testing.T) {
	if severityName(SeverityCritical) != "critical" {
		t.Fail()
	}
	if severityName(SeverityDegraded) != "degraded" {
		t.Fail()
	}
	if severityName(Severity(99)) != "unknown" {
		t.Fail()
	}
}

func TestRegistry_Start_IsIdempotent(t *testing.T) {
	r := NewRegistry(WithCacheTTL(10 * time.Millisecond))
	r.Register(&stubChecker{name: "x", sev: SeverityCritical})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx)
	// Second Start should no-op (CompareAndSwap returns false)
	r.Start(ctx)
	// Goroutine will exit when ctx is cancelled
}

func TestReadiness_BodyDoesNotLeakCheckNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chk := &stubChecker{name: "very-internal-postgres", sev: SeverityCritical}
	r := NewRegistry()
	r.Register(chk)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/readyz", nil)
	r.Readiness(c)
	if strings.Contains(w.Body.String(), "very-internal-postgres") {
		t.Fatalf("public readiness leaked checker name: %s", w.Body.String())
	}
}
