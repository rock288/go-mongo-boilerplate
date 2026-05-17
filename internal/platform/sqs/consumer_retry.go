package sqs

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// ConsumeRetry consumes the -retry queue and re-publishes each message back
// to its origin (suffix-stripped). The per-message delay was already applied
// at enqueue time via Producer.WithDelay, so the message only becomes visible
// in the retry queue after the backoff window has elapsed — this consumer's
// job is purely to move messages back to the main queue without further sleep.
//
// Retry count is preserved so the main consumer's MaxRetries check continues
// to apply. Same shutdown semantics as Consume.
func ConsumeRetry(ctx context.Context, client SQSAPI, producer Producer,
	cfg config.SQSConsumerConfig, logger *slog.Logger, heartbeat HeartbeatFunc) error {
	cfg = defaultsForConsumer(cfg)

	retryQueueURL := destQueueURL(cfg.QueueURL, cfg.RetrySuffix, cfg.DLQSuffix, false)
	mainQueueURL := strings.TrimSuffix(retryQueueURL, cfg.RetrySuffix)

	handler := MessageHandlerFunc(func(ctx context.Context, msg *types.Message) error {
		retryCount := RetryCount(msg)

		body := ""
		if msg.Body != nil {
			body = *msg.Body
		}
		attrs := SanitizeForRepublish(msg)
		// Preserve retry count so main consumer's budget check still trips.
		SetStringAttribute(attrs, HeaderRetryCount, strconv.Itoa(retryCount))

		pubOpts := []PublishOpt{ForceSample()}
		if k := GetStringAttribute(attrs, HeaderIdempotencyKey); k != "" {
			pubOpts = append(pubOpts, WithIdempotencyKey(k))
		}
		for k, v := range attrs {
			if k == HeaderTraceParent || k == HeaderTraceState || k == HeaderIdempotencyKey {
				continue
			}
			if v.StringValue != nil {
				pubOpts = append(pubOpts, WithAttribute(k, *v.StringValue))
			}
		}
		return producer.Publish(ctx, mainQueueURL, body, pubOpts...)
	})

	retryConsumerCfg := cfg
	retryConsumerCfg.QueueURL = retryQueueURL
	// Retry consumer's own retry/DLQ flow is disabled — if republish to main
	// fails, the message stays in the retry queue (SQS visibility timeout
	// redrives). Setting MaxRetries=0 so any handler error routes to DLQ on
	// the retry queue itself, but the handler above doesn't return retry-able
	// errors — only producer publish failures, which we let surface so
	// in-flight stays on retry queue for retry next poll.
	return Consume(ctx, client, handler, logger, ConsumeOptions{
		Consumer:  retryConsumerCfg,
		Producer:  nil, // disables nested retry; producer republish failures bubble up
		Heartbeat: heartbeat,
	})
}
