package resilience

import (
	"math/rand"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"
)

// BreakerConfig parameterizes a circuit breaker.
type BreakerConfig struct {
	Name             string
	MaxRequests      uint32        // half-open allowed concurrent probes
	Interval         time.Duration // rolling-window for counts
	Timeout          time.Duration // open → half-open delay (jittered)
	FailureThreshold uint32        // consecutive failures to trip
}

// DefaultBreaker returns sane defaults for a named breaker.
func DefaultBreaker(name string) BreakerConfig {
	return BreakerConfig{
		Name:             name,
		MaxRequests:      1,
		Interval:         60 * time.Second,
		Timeout:          60 * time.Second,
		FailureThreshold: 5,
	}
}

// Registry holds named breakers so callers can fetch the same instance.
type Registry struct {
	mu       sync.RWMutex
	breakers map[string]any
}

// NewRegistry returns an empty breaker registry.
func NewRegistry() *Registry {
	return &Registry{breakers: map[string]any{}}
}

// Breaker returns the singleton CircuitBreaker for name, creating it on first
// call with the supplied config. Jitter is applied to Timeout to desync
// half-open windows across replicas.
func Breaker[T any](r *Registry, cfg BreakerConfig) *gobreaker.CircuitBreaker[T] {
	r.mu.RLock()
	if cb, ok := r.breakers[cfg.Name]; ok {
		r.mu.RUnlock()
		return cb.(*gobreaker.CircuitBreaker[T])
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if cb, ok := r.breakers[cfg.Name]; ok {
		return cb.(*gobreaker.CircuitBreaker[T])
	}

	timeout := cfg.Timeout
	if timeout > 0 {
		// ±20% jitter — desync half-open windows across replicas. Non-crypto OK.
		jitter := time.Duration(rand.Int63n(int64(timeout) / 5)) //#nosec G404 -- jitter, not security-sensitive
		timeout = timeout - timeout/10 + jitter
	}
	threshold := cfg.FailureThreshold
	if threshold == 0 {
		threshold = 5
	}
	cb := gobreaker.NewCircuitBreaker[T](gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     timeout,
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= threshold
		},
	})
	r.breakers[cfg.Name] = cb
	return cb
}
