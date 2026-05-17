package resilience

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v5"
)

// RetryConfig parameterizes the retry helper.
type RetryConfig struct {
	MaxTries        uint
	InitialInterval time.Duration
	MaxInterval     time.Duration
	MaxElapsedTime  time.Duration
}

// DefaultRetry returns sane defaults: 3 tries, 100ms→2s with jitter.
func DefaultRetry() RetryConfig {
	return RetryConfig{
		MaxTries:        3,
		InitialInterval: 100 * time.Millisecond,
		MaxInterval:     2 * time.Second,
		MaxElapsedTime:  30 * time.Second,
	}
}

// Do runs op with exponential backoff (jittered). Non-retryable errors must
// be wrapped with backoff.Permanent — these halt the loop immediately.
// ctx cancellation also halts.
func Do[T any](ctx context.Context, cfg RetryConfig, op func() (T, error)) (T, error) {
	exp := backoff.NewExponentialBackOff()
	if cfg.InitialInterval > 0 {
		exp.InitialInterval = cfg.InitialInterval
	}
	if cfg.MaxInterval > 0 {
		exp.MaxInterval = cfg.MaxInterval
	}
	return backoff.Retry(ctx, op,
		backoff.WithBackOff(exp),
		backoff.WithMaxTries(cfg.MaxTries),
		backoff.WithMaxElapsedTime(cfg.MaxElapsedTime),
	)
}

// Permanent wraps err so the retry loop stops immediately.
func Permanent(err error) error { return backoff.Permanent(err) }
