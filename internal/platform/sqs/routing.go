package sqs

import (
	"context"
	"errors"
)

// Route is the dispatch decision for a consumed message after the handler returns.
type Route int

const (
	// RouteCommit — handler succeeded; delete the message.
	RouteCommit Route = iota
	// RouteDrop — ctx cancelled during shutdown; do NOT delete, do NOT republish.
	// SQS will redeliver after visibility timeout.
	RouteDrop
	// RouteRetry — transient handler error; republish to the .retry queue with backoff.
	RouteRetry
	// RouteDLQ — permanent failure (ErrNonRetryable or retry budget exhausted).
	RouteDLQ
)

// decideRoute is a pure function — exhaustively unit-tested. Mirror of
// kafka.decideRoute so the routing semantics are identical across brokers.
func decideRoute(err error, retryCount, maxRetries int) Route {
	if err == nil {
		return RouteCommit
	}
	if errors.Is(err, context.Canceled) {
		return RouteDrop
	}
	if errors.Is(err, ErrNonRetryable) {
		return RouteDLQ
	}
	if retryCount >= maxRetries {
		return RouteDLQ
	}
	return RouteRetry
}
