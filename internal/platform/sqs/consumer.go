package sqs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

const consumerTracer = "github.com/rock288/go-mongo-boilerplate/internal/platform/sqs.consumer"

// MessageHandler implementations must:
//   - return nil on success
//   - return errors.Is(err, context.Canceled) when ctx is cancelled (no republish)
//   - return errors.Is(err, ErrNonRetryable) to skip retries
type MessageHandler interface {
	Handle(ctx context.Context, msg *types.Message) error
}

// MessageHandlerFunc adapts a func to MessageHandler.
type MessageHandlerFunc func(context.Context, *types.Message) error

func (f MessageHandlerFunc) Handle(ctx context.Context, m *types.Message) error { return f(ctx, m) }

// HeartbeatFunc is called after each successful ReceiveMessage (even empty polls).
type HeartbeatFunc func()

// ConsumeOptions controls the main consumer loop.
type ConsumeOptions struct {
	Consumer  config.SQSConsumerConfig
	Producer  Producer      // for retry/DLQ republish
	Heartbeat HeartbeatFunc // optional
	OnDone    func()        // optional — fires when the loop exits (WaitGroup signal)
}

// Consume runs the long-poll main consumer loop on cfg.QueueURL. Commit semantics:
//   - handler success                              → DeleteMessage (ACK)
//   - errors.Is(err, context.Canceled) (shutdown)  → no ACK, no republish
//   - ErrNonRetryable OR retry >= MaxRetries       → publish to -dlq, then DeleteMessage
//   - other err                                    → publish to -retry queue with backoff,
//     then DeleteMessage
//
// Publish-then-delete is non-atomic; on the rare crash window between the two,
// the original message redrives via SQS visibility timeout (consumers should
// dedupe via x-idempotency-key).
func Consume(ctx context.Context, client SQSAPI, h MessageHandler, logger *slog.Logger, opts ConsumeOptions) error {
	if opts.OnDone != nil {
		defer opts.OnDone()
	}
	cfg := defaultsForConsumer(opts.Consumer)
	tracer := otel.Tracer(consumerTracer)
	qname, _ := QueueName(cfg.QueueURL)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		out, err := client.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
			QueueUrl:              aws.String(cfg.QueueURL),
			MaxNumberOfMessages:   cfg.MaxMessages,
			WaitTimeSeconds:       cfg.WaitTimeSeconds,
			VisibilityTimeout:     cfg.VisibilityTimeoutSeconds,
			MessageAttributeNames: []string{"All"},
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			logger.Error("sqs receive error", slog.String("queue", qname), slog.String("error", err.Error()))
			// Brief backoff so a broken endpoint doesn't spin.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		}
		if opts.Heartbeat != nil {
			opts.Heartbeat()
		}

		for i := range out.Messages {
			msg := &out.Messages[i]
			if processMessage(ctx, client, h, opts.Producer, msg, cfg, qname, tracer, logger); ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
}

// processMessage runs one handler dispatch + routing decision + commit/republish.
// Returns nothing — observable failure is logged. Caller checks ctx.Err() after.
func processMessage(
	ctx context.Context,
	client SQSAPI,
	h MessageHandler,
	producer Producer,
	msg *types.Message,
	cfg config.SQSConsumerConfig,
	qname string,
	tracer trace.Tracer,
	logger *slog.Logger,
) {
	handlerCtx := ExtractTraceContext(ctx, msg)
	handlerCtx, span := tracer.Start(handlerCtx, "sqs.consume "+qname,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "aws_sqs"),
			attribute.String("messaging.operation", "receive"),
			attribute.String("messaging.source.name", qname),
		),
	)
	if msg.MessageId != nil {
		span.SetAttributes(attribute.String("messaging.message.id", *msg.MessageId))
	}

	err := h.Handle(handlerCtx, msg)
	retryCount := RetryCount(msg)
	route := decideRoute(err, retryCount, cfg.MaxRetries)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.SetAttributes(attribute.Int("messaging.aws_sqs.retry_count", retryCount))
	span.End()

	switch route {
	case RouteCommit:
		deleteMessage(ctx, client, cfg.QueueURL, msg, logger)
	case RouteDrop:
		logger.Warn("sqs handler cancelled during shutdown — no republish",
			slog.String("queue", qname),
			slog.String("message_id", safeID(msg)),
		)
	case RouteRetry, RouteDLQ:
		toDLQ := route == RouteDLQ
		dest := destQueueURL(cfg.QueueURL, cfg.RetrySuffix, cfg.DLQSuffix, toDLQ)
		if produceErr := republish(ctx, producer, msg, dest, err, cfg, toDLQ); produceErr != nil {
			logger.Error("sqs republish failed — leaving message for redrive",
				slog.String("dest", dest),
				slog.String("error", produceErr.Error()),
			)
			return // do NOT delete; SQS visibility timeout will redeliver
		}
		logger.Info("sqs message routed",
			slog.String("dest", dest),
			slog.String("origin", qname),
			slog.String("message_id", safeID(msg)),
			slog.Int("retry_count", retryCount),
			slog.String("error", SanitizeError(err)),
		)
		deleteMessage(ctx, client, cfg.QueueURL, msg, logger)
	}
}

func deleteMessage(ctx context.Context, client SQSAPI, queueURL string, msg *types.Message, logger *slog.Logger) {
	if msg.ReceiptHandle == nil {
		return
	}
	if _, err := client.DeleteMessage(ctx, &awssqs.DeleteMessageInput{
		QueueUrl:      aws.String(queueURL),
		ReceiptHandle: msg.ReceiptHandle,
	}); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("sqs delete failed", slog.String("error", err.Error()), slog.String("message_id", safeID(msg)))
	}
}

func safeID(msg *types.Message) string {
	if msg == nil || msg.MessageId == nil {
		return ""
	}
	return *msg.MessageId
}

func defaultsForConsumer(cfg config.SQSConsumerConfig) config.SQSConsumerConfig {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetrySuffix == "" {
		cfg.RetrySuffix = "-retry"
	}
	if cfg.DLQSuffix == "" {
		cfg.DLQSuffix = "-dlq"
	}
	if cfg.VisibilityTimeoutSeconds <= 0 {
		cfg.VisibilityTimeoutSeconds = 30
	}
	if cfg.WaitTimeSeconds <= 0 {
		cfg.WaitTimeSeconds = 20
	}
	if cfg.MaxMessages <= 0 || cfg.MaxMessages > 10 {
		cfg.MaxMessages = 10
	}
	if cfg.DLQMaxPayloadBytes <= 0 {
		cfg.DLQMaxPayloadBytes = 65536
	}
	if cfg.RetryBackoffBase <= 0 {
		cfg.RetryBackoffBase = 5 * time.Second
	}
	if cfg.RetryBackoffMax <= 0 {
		cfg.RetryBackoffMax = SQSMaxDelay
	}
	return cfg
}

// destQueueURL derives the retry-queue or DLQ URL from the origin URL by
// stripping retry suffix (if present) from the queue name and appending
// either the retry or DLQ suffix.
func destQueueURL(originURL, retrySuffix, dlqSuffix string, toDLQ bool) string {
	idx := strings.LastIndex(originURL, "/")
	if idx < 0 {
		return originURL // malformed; let SendMessage fail with a clear error
	}
	prefix, name := originURL[:idx+1], originURL[idx+1:]
	base := strings.TrimSuffix(name, retrySuffix)
	if toDLQ {
		return prefix + base + dlqSuffix
	}
	return prefix + base + retrySuffix
}

// republish sends src to dest (retry or DLQ) with sanitized attributes +
// force-sampled span. Bounded inline retry: 3 attempts with 100ms/500ms/1s
// backoff; aborts immediately on ctx.Err().
func republish(ctx context.Context, p Producer, src *types.Message, dest string,
	handlerErr error, cfg config.SQSConsumerConfig, toDLQ bool) error {
	if p == nil {
		return fmt.Errorf("sqs: producer not configured for retry/DLQ")
	}
	attrs := SanitizeForRepublish(src)
	body := ""
	if src.Body != nil {
		body = *src.Body
	}
	if toDLQ {
		if len(body) > cfg.DLQMaxPayloadBytes {
			body = body[:cfg.DLQMaxPayloadBytes]
		}
		SetErrorReason(attrs, handlerErr)
	} else {
		IncRetryCount(attrs)
	}

	pubOpts := []PublishOpt{ForceSample()}
	if k := GetStringAttribute(attrs, HeaderIdempotencyKey); k != "" {
		pubOpts = append(pubOpts, WithIdempotencyKey(k))
	}
	// Forward custom attrs (sanitize already stripped reserved/blacklisted keys
	// that the producer will re-inject).
	for k, v := range attrs {
		if k == HeaderTraceParent || k == HeaderTraceState || k == HeaderIdempotencyKey {
			continue
		}
		if v.StringValue != nil {
			pubOpts = append(pubOpts, WithAttribute(k, *v.StringValue))
		}
	}
	// On retry path, apply per-message backoff delay so the message becomes
	// invisible until the delay window elapses.
	if !toDLQ {
		retryCount := RetryCount(src) + 1
		delay := backoffDelay(cfg.RetryBackoffBase, cfg.RetryBackoffMax, retryCount-1)
		if delay > 0 {
			pubOpts = append(pubOpts, WithDelay(delay))
		}
	}

	const attempts = 3
	backoff := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, time.Second}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := p.Publish(ctx, dest, body, pubOpts...); err == nil {
			return nil
		} else {
			lastErr = err
			if ctx.Err() != nil {
				return lastErr
			}
			time.Sleep(backoff[i])
		}
	}
	return lastErr
}
