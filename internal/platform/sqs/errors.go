package sqs

import "errors"

// ErrNonRetryable signals a handler error that should bypass the retry queue
// and go straight to DLQ. Mirror of kafka.ErrNonRetryable.
var ErrNonRetryable = errors.New("sqs: non-retryable handler error")

// ErrTooManyAttributes is returned by the producer when the caller passes
// more custom MessageAttributes than the SQS hard limit allows after
// reserving the 3 trace/idempotency keys.
var ErrTooManyAttributes = errors.New("sqs: too many message attributes")
