package kafka

import (
	"context"
	"errors"
	"testing"
)

func TestDecideRoute(t *testing.T) {
	t.Parallel()

	someErr := errors.New("transient")

	tests := []struct {
		name       string
		err        error
		retryCount int
		maxRetries int
		want       Route
	}{
		{
			name: "nil_error_commits",
			err:  nil,
			want: RouteCommit,
		},
		{
			name: "context_cancelled_drops",
			err:  context.Canceled,
			want: RouteDrop,
		},
		{
			name: "context_cancelled_wrapped_drops",
			err:  errors.Join(errors.New("outer"), context.Canceled),
			want: RouteDrop,
		},
		{
			name: "non_retryable_goes_to_dlq",
			err:  ErrNonRetryable,
			want: RouteDLQ,
		},
		{
			name: "non_retryable_wrapped_goes_to_dlq",
			err:  errors.Join(errors.New("bad payload"), ErrNonRetryable),
			want: RouteDLQ,
		},
		{
			name:       "retry_count_below_max_retries",
			err:        someErr,
			retryCount: 1,
			maxRetries: 3,
			want:       RouteRetry,
		},
		{
			name:       "retry_count_zero_retries",
			err:        someErr,
			retryCount: 0,
			maxRetries: 3,
			want:       RouteRetry,
		},
		{
			name:       "retry_count_equals_max_goes_to_dlq",
			err:        someErr,
			retryCount: 3,
			maxRetries: 3,
			want:       RouteDLQ,
		},
		{
			name:       "retry_count_exceeds_max_goes_to_dlq",
			err:        someErr,
			retryCount: 5,
			maxRetries: 3,
			want:       RouteDLQ,
		},
		{
			name:       "context_canceled_wins_over_max_retries",
			err:        context.Canceled,
			retryCount: 99,
			maxRetries: 3,
			want:       RouteDrop,
		},
		{
			name:       "non_retryable_wins_over_under_max",
			err:        ErrNonRetryable,
			retryCount: 0,
			maxRetries: 3,
			want:       RouteDLQ,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := decideRoute(tt.err, tt.retryCount, tt.maxRetries)
			if got != tt.want {
				t.Fatalf("decideRoute(%v, %d, %d) = %v, want %v",
					tt.err, tt.retryCount, tt.maxRetries, got, tt.want)
			}
		})
	}
}
