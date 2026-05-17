package health

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// Severity controls how a failing checker affects readiness.
type Severity int

const (
	SeverityCritical Severity = iota // failing → not ready
	SeverityDegraded                 // failing → ready=true with details
)

// Checker is the contract for a single health probe.
type Checker interface {
	Name() string
	Severity() Severity
	Check(ctx context.Context) error
}

// Result is the cached outcome of a single checker.
type Result struct {
	Name      string
	Severity  Severity
	Err       error
	UpdatedAt time.Time
	// ConsecutiveFails counts uninterrupted failure cycles. Reset on success.
	ConsecutiveFails int
}

// Registry caches results, runs checks asynchronously, and exposes Gin
// handlers for /healthz (liveness) and /readyz (readiness).
type Registry struct {
	checkers     []Checker
	cacheTTL     time.Duration
	failureLimit int
	timeout      time.Duration

	mu      sync.RWMutex
	results map[string]*Result
	started atomic.Bool
}

// Option configures a Registry.
type Option func(*Registry)

// WithCacheTTL sets how stale a result may be before it is recomputed.
func WithCacheTTL(d time.Duration) Option { return func(r *Registry) { r.cacheTTL = d } }

// WithFailureLimit sets how many consecutive failures of a critical checker
// flip /readyz to 503.
func WithFailureLimit(n int) Option { return func(r *Registry) { r.failureLimit = n } }

// WithCheckTimeout caps how long a single Check call may run.
func WithCheckTimeout(d time.Duration) Option { return func(r *Registry) { r.timeout = d } }

// NewRegistry creates a Registry with sensible defaults.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{
		cacheTTL:     5 * time.Second,
		failureLimit: 3,
		timeout:      2 * time.Second,
		results:      map[string]*Result{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register adds a checker. Safe to call before Start.
func (r *Registry) Register(c Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers = append(r.checkers, c)
	r.results[c.Name()] = &Result{Name: c.Name(), Severity: c.Severity()}
}

// Start spawns a background refresher that runs each checker every cacheTTL.
// Returns immediately. The goroutine exits when ctx is cancelled.
func (r *Registry) Start(ctx context.Context) {
	if !r.started.CompareAndSwap(false, true) {
		return
	}
	go func() {
		ticker := time.NewTicker(r.cacheTTL)
		defer ticker.Stop()
		r.runAll(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.runAll(ctx)
			}
		}
	}()
}

func (r *Registry) runAll(ctx context.Context) {
	r.mu.RLock()
	checkers := append([]Checker(nil), r.checkers...)
	r.mu.RUnlock()
	for _, c := range checkers {
		cctx, cancel := context.WithTimeout(ctx, r.timeout)
		err := c.Check(cctx)
		cancel()
		r.mu.Lock()
		res := r.results[c.Name()]
		res.UpdatedAt = time.Now()
		res.Err = err
		if err == nil {
			res.ConsecutiveFails = 0
		} else {
			res.ConsecutiveFails++
		}
		r.mu.Unlock()
	}
}

// Snapshot copies current results for inspection.
func (r *Registry) Snapshot() []Result {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Result, 0, len(r.results))
	for _, res := range r.results {
		out = append(out, *res)
	}
	return out
}

// Liveness returns 200 always (process alive).
func (r *Registry) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readiness returns 200 with {"status":"ok"} when no critical checker is in
// failure beyond the configured threshold, 503 with {"status":"not_ready"}
// otherwise. Per-check detail is NOT exposed here.
func (r *Registry) Readiness(c *gin.Context) {
	if r.ready() {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
		return
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
}

func (r *Registry) ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, res := range r.results {
		if res.Severity != SeverityCritical {
			continue
		}
		if res.Err != nil && res.ConsecutiveFails >= r.failureLimit {
			return false
		}
	}
	return true
}

// DetailedReadiness exposes per-check breakdown. Bind this on an internal
// network only — body contents reveal infrastructure topology.
func (r *Registry) DetailedReadiness(c *gin.Context) {
	snap := r.Snapshot()
	checks := make([]gin.H, 0, len(snap))
	for _, res := range snap {
		entry := gin.H{
			"name":              res.Name,
			"severity":          severityName(res.Severity),
			"ok":                res.Err == nil,
			"consecutive_fails": res.ConsecutiveFails,
			"updated_at":        res.UpdatedAt.UTC().Format(time.RFC3339),
		}
		if res.Err != nil {
			entry["error"] = res.Err.Error()
		}
		checks = append(checks, entry)
	}
	status := http.StatusOK
	body := gin.H{"status": "ok", "checks": checks}
	if !r.ready() {
		status = http.StatusServiceUnavailable
		body["status"] = "not_ready"
	}
	c.JSON(status, body)
}

func severityName(s Severity) string {
	switch s {
	case SeverityCritical:
		return "critical"
	case SeverityDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}
