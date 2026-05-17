package resilience

import (
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_Breaker_ReturnsSameInstance(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cfg := BreakerConfig{Name: "test1", FailureThreshold: 2, Timeout: 100 * time.Millisecond}
	cb1 := Breaker[string](r, cfg)
	cb2 := Breaker[string](r, cfg)
	require.Same(t, cb1, cb2, "same name should return the same breaker instance")
}

func TestRegistry_Breaker_DifferentNamesAreSeparate(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cb1 := Breaker[string](r, BreakerConfig{Name: "a", FailureThreshold: 1})
	cb2 := Breaker[string](r, BreakerConfig{Name: "b", FailureThreshold: 1})
	require.NotSame(t, cb1, cb2)
}

func TestRegistry_Breaker_TripsAfterThreshold(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cb := Breaker[string](r, BreakerConfig{
		Name:             "trip-test",
		FailureThreshold: 3,
		Timeout:          200 * time.Millisecond,
	})
	require.Equal(t, gobreaker.StateClosed, cb.State())

	someErr := errors.New("fail")
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(func() (string, error) { return "", someErr })
		require.ErrorIs(t, err, someErr)
	}
	assert.Equal(t, gobreaker.StateOpen, cb.State(), "breaker should be open after 3 failures")

	_, err := cb.Execute(func() (string, error) { return "ok", nil })
	require.ErrorIs(t, err, gobreaker.ErrOpenState)
}

func TestRegistry_Breaker_HalfOpenThenClosedOnSuccess(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cb := Breaker[string](r, BreakerConfig{
		Name:             "recover-test",
		FailureThreshold: 1,
		MaxRequests:      1,
		Timeout:          50 * time.Millisecond,
	})

	someErr := errors.New("fail")
	_, _ = cb.Execute(func() (string, error) { return "", someErr })
	require.Equal(t, gobreaker.StateOpen, cb.State())

	// Wait for timeout (max jittered 50ms + 20% buffer) — gobreaker checks the
	// timer lazily on the next Execute, so we poll instead of sleeping a fixed
	// duration.
	deadline := time.Now().Add(500 * time.Millisecond)
	var got string
	var execErr error
	for time.Now().Before(deadline) {
		got, execErr = cb.Execute(func() (string, error) { return "ok", nil })
		if execErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, execErr, "breaker should eventually move through half-open and succeed")
	assert.Equal(t, "ok", got)
	assert.Equal(t, gobreaker.StateClosed, cb.State(), "successful half-open probe should close breaker")
}

func TestRegistry_Breaker_DefaultThresholdAppliedWhenZero(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	cb := Breaker[string](r, BreakerConfig{Name: "default-threshold"})
	// Default threshold is 5; below that we should remain closed
	someErr := errors.New("fail")
	for i := 0; i < 4; i++ {
		_, _ = cb.Execute(func() (string, error) { return "", someErr })
	}
	assert.Equal(t, gobreaker.StateClosed, cb.State())

	// 5th consecutive failure trips
	_, _ = cb.Execute(func() (string, error) { return "", someErr })
	assert.Equal(t, gobreaker.StateOpen, cb.State())
}

func TestDefaultBreaker_HasSaneDefaults(t *testing.T) {
	t.Parallel()
	cfg := DefaultBreaker("svc")
	assert.Equal(t, "svc", cfg.Name)
	assert.Equal(t, uint32(1), cfg.MaxRequests)
	assert.Equal(t, uint32(5), cfg.FailureThreshold)
	assert.Equal(t, 60*time.Second, cfg.Interval)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
}

func TestRegistry_Breaker_ConcurrentLookupSafe(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	cfg := BreakerConfig{Name: "concurrent", FailureThreshold: 1}

	done := make(chan *gobreaker.CircuitBreaker[string], 10)
	for i := 0; i < 10; i++ {
		go func() {
			done <- Breaker[string](r, cfg)
		}()
	}
	first := <-done
	for i := 0; i < 9; i++ {
		got := <-done
		require.Same(t, first, got)
	}
}
