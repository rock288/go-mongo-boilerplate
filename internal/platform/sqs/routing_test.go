package sqs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecideRoute(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		err        error
		retryCount int
		maxRetries int
		want       Route
	}{
		{"nil err → commit", nil, 0, 3, RouteCommit},
		{"nil err high count → commit", nil, 99, 3, RouteCommit},
		{"ctx cancelled → drop", context.Canceled, 0, 3, RouteDrop},
		{"generic err with budget → retry", errors.New("transient"), 0, 3, RouteRetry},
		{"non-retryable → DLQ", ErrNonRetryable, 0, 3, RouteDLQ},
		{"non-retryable wrapped → DLQ", errors.Join(errors.New("wrap"), ErrNonRetryable), 0, 3, RouteDLQ},
		{"retry budget left → retry", errors.New("boom"), 0, 3, RouteRetry},
		{"at retry budget → DLQ", errors.New("boom"), 3, 3, RouteDLQ},
		{"over retry budget → DLQ", errors.New("boom"), 5, 3, RouteDLQ},
		{"zero max with err → DLQ", errors.New("boom"), 0, 0, RouteDLQ},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := decideRoute(tt.err, tt.retryCount, tt.maxRetries)
			assert.Equal(t, tt.want, got)
		})
	}
}
