package kafka

import (
	"context"
	"errors"
)

// Route classifies what should happen to a consumed record after handler returns.
type Route int

const (
	// RouteCommit — handler succeeded; mark record committable.
	RouteCommit Route = iota
	// RouteRetry — retryable failure; republish to retry topic.
	RouteRetry
	// RouteDLQ — non-retryable or retries exhausted; republish to DLQ.
	RouteDLQ
	// RouteDrop — context cancelled during shutdown; no commit, no republish.
	RouteDrop
)

// decideRoute is the pure routing decision extracted from the consumer loop.
// Inputs: handler error, current retry count from record header, configured max.
// Pure — no I/O. Easy to unit-test exhaustively.
func decideRoute(err error, retryCount, maxRetries int) Route {
	if err == nil {
		return RouteCommit
	}
	if errors.Is(err, context.Canceled) {
		return RouteDrop
	}
	if errors.Is(err, ErrNonRetryable) || retryCount >= maxRetries {
		return RouteDLQ
	}
	return RouteRetry
}
