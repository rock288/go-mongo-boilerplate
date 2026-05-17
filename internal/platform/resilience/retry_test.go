package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_RetriesThenSucceeds(t *testing.T) {
	calls := 0
	got, err := Do[int](context.Background(), RetryConfig{MaxTries: 5, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond}, func() (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_PermanentErrorHaltsImmediately(t *testing.T) {
	calls := 0
	_, err := Do[int](context.Background(), RetryConfig{MaxTries: 5, InitialInterval: time.Millisecond}, func() (int, error) {
		calls++
		return 0, Permanent(errors.New("dead"))
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (permanent), got %d", calls)
	}
}

func TestDefaultRetry_HasExpectedDefaults(t *testing.T) {
	cfg := DefaultRetry()
	if cfg.MaxTries != 3 {
		t.Errorf("MaxTries: want 3, got %d", cfg.MaxTries)
	}
	if cfg.InitialInterval != 100*time.Millisecond {
		t.Errorf("InitialInterval: want 100ms, got %v", cfg.InitialInterval)
	}
	if cfg.MaxInterval != 2*time.Second {
		t.Errorf("MaxInterval: want 2s, got %v", cfg.MaxInterval)
	}
	if cfg.MaxElapsedTime != 30*time.Second {
		t.Errorf("MaxElapsedTime: want 30s, got %v", cfg.MaxElapsedTime)
	}
}

func TestDo_ContextCancelHalts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	_, err := Do[int](ctx, RetryConfig{MaxTries: 5, InitialInterval: time.Millisecond}, func() (int, error) {
		calls++
		return 0, errors.New("transient")
	})
	if err == nil {
		t.Fatal("expected ctx error")
	}
}
