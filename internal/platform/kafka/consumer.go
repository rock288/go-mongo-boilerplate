package kafka

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// ErrNonRetryable signals a handler error that should bypass the retry topic
// and go straight to DLQ.
var ErrNonRetryable = errors.New("kafka: non-retryable handler error")

// MessageHandler implementations must:
//   - return nil on success
//   - return errors.Is(err, context.Canceled) when ctx is cancelled (no republish)
//   - return errors.Is(err, ErrNonRetryable) to skip retries
type MessageHandler interface {
	Handle(ctx context.Context, record *kgo.Record) error
}

// MessageHandlerFunc adapts a func to MessageHandler.
type MessageHandlerFunc func(context.Context, *kgo.Record) error

func (f MessageHandlerFunc) Handle(ctx context.Context, r *kgo.Record) error { return f(ctx, r) }

// HeartbeatFunc is called after each successful PollFetches.
type HeartbeatFunc func()

const consumerTracer = "github.com/rock288/go-mongo-boilerplate/internal/platform/kafka.consumer"

// ConsumeOptions controls the main consumer loop.
type ConsumeOptions struct {
	Consumer  config.KafkaConsumerConfig
	Producer  Producer // used for republish to retry/DLQ topics
	Heartbeat HeartbeatFunc
	OnDone    func() // called when the loop exits (e.g., for WaitGroup signaling)
}

// Consume runs the main consumer loop. Commit semantics:
//   - handler success → record marked committable
//   - handler error + ctx cancelled during shutdown → break, no republish, no commit
//   - handler error + ErrNonRetryable OR retryCount >= MaxRetries → DLQ; commit on success
//   - handler error otherwise → retry topic with incremented count; commit on success
//   - produce(retry/DLQ) fails after bounded retries → don't commit, re-fetch next iteration
func Consume(ctx context.Context, client *kgo.Client, h MessageHandler, logger *slog.Logger, opts ConsumeOptions) error {
	if opts.OnDone != nil {
		defer opts.OnDone()
	}
	cfg := opts.Consumer
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetrySuffix == "" {
		cfg.RetrySuffix = ".retry"
	}
	if cfg.DLQSuffix == "" {
		cfg.DLQSuffix = ".dlq"
	}
	if cfg.DLQMaxPayloadBytes <= 0 {
		cfg.DLQMaxPayloadBytes = 65536
	}
	tracer := otel.Tracer(consumerTracer)
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		fetches := client.PollFetches(ctx)
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, fe := range errs {
				if errors.Is(fe.Err, context.Canceled) {
					return fe.Err
				}
				logger.Error("kafka fetch error",
					slog.String("topic", fe.Topic),
					slog.Int("partition", int(fe.Partition)),
					slog.String("error", fe.Err.Error()),
				)
			}
		}
		if opts.Heartbeat != nil {
			opts.Heartbeat()
		}

		var committable []*kgo.Record
		stopProcessing := false
		iter := fetches.RecordIter()
		for !iter.Done() && !stopProcessing {
			record := iter.Next()
			handlerCtx := ExtractTraceContext(ctx, record)
			handlerCtx, span := tracer.Start(handlerCtx, "kafka.consume "+record.Topic,
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(
					attribute.String("messaging.system", "kafka"),
					attribute.String("messaging.source", record.Topic),
					attribute.Int64("messaging.kafka.offset", record.Offset),
				),
			)

			wg.Add(1)
			err := func() error {
				defer wg.Done()
				return h.Handle(handlerCtx, record)
			}()

			retryCount := RetryCount(record)
			route := decideRoute(err, retryCount, cfg.MaxRetries)

			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()

			switch route {
			case RouteCommit:
				committable = append(committable, record)
				continue
			case RouteDrop:
				logger.Warn("handler cancelled during shutdown — no republish",
					slog.String("topic", record.Topic),
					slog.Int64("offset", record.Offset),
				)
				stopProcessing = true
			case RouteRetry, RouteDLQ:
				toDLQ := route == RouteDLQ
				destTopic := destTopic(record.Topic, cfg, toDLQ)
				if produceErr := republish(ctx, opts.Producer, record, destTopic, err, cfg, toDLQ); produceErr != nil {
					logger.Error("kafka republish failed — stopping batch, will retry",
						slog.String("dest", destTopic),
						slog.String("error", produceErr.Error()),
					)
					stopProcessing = true
					break
				}
				logger.Info("kafka record routed",
					slog.String("dest", destTopic),
					slog.String("origin", record.Topic),
					slog.Int64("offset", record.Offset),
					slog.Int("retry_count", retryCount),
					slog.String("error", SanitizeError(err)),
				)
				committable = append(committable, record)
			}
			if stopProcessing {
				break
			}
		}

		if len(committable) > 0 {
			if err := client.CommitRecords(ctx, committable...); err != nil {
				logger.Error("kafka commit failed", slog.String("error", err.Error()))
			}
		}
	}
}

func destTopic(origin string, cfg config.KafkaConsumerConfig, toDLQ bool) string {
	base := strings.TrimSuffix(origin, cfg.RetrySuffix)
	if toDLQ {
		return base + cfg.DLQSuffix
	}
	return base + cfg.RetrySuffix
}

func republish(ctx context.Context, p Producer, src *kgo.Record, dest string, handlerErr error, cfg config.KafkaConsumerConfig, toDLQ bool) error {
	if p == nil {
		return errors.New("kafka: producer not configured for retry/DLQ")
	}
	sanitized := SanitizeForRepublish(src, dest)
	if toDLQ {
		if len(sanitized.Value) > cfg.DLQMaxPayloadBytes {
			sanitized.Value = sanitized.Value[:cfg.DLQMaxPayloadBytes]
		}
		SetErrorReason(sanitized, handlerErr)
	} else {
		IncRetryCount(sanitized)
	}

	pubOpts := []PublishOpt{ForceSample()}
	if k := GetIdempotencyKey(sanitized); k != "" {
		pubOpts = append(pubOpts, WithIdempotencyKey(k))
	}
	for _, h := range sanitized.Headers {
		if h.Key == HeaderIdempotencyKey || h.Key == HeaderTraceParent || h.Key == HeaderTraceState {
			continue
		}
		pubOpts = append(pubOpts, WithHeader(h.Key, h.Value))
	}

	const attempts = 3
	backoff := []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, time.Second}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := p.Publish(ctx, dest, sanitized.Key, sanitized.Value, pubOpts...); err == nil {
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

// ConsumeRetry consumes the .retry topic, sleeps the backoff, and republishes
// to the original topic (suffix-stripped). Same shutdown semantics as Consume.
func ConsumeRetry(ctx context.Context, client *kgo.Client, producer Producer, cfg config.KafkaConsumerConfig, logger *slog.Logger, heartbeat HeartbeatFunc) error {
	if cfg.RetryBackoffBase <= 0 {
		cfg.RetryBackoffBase = time.Second
	}
	if cfg.RetryBackoffMax <= 0 {
		cfg.RetryBackoffMax = 5 * time.Minute
	}

	handler := MessageHandlerFunc(func(ctx context.Context, record *kgo.Record) error {
		retryCount := RetryCount(record)
		delay := backoffDelay(cfg.RetryBackoffBase, cfg.RetryBackoffMax, retryCount)

		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}

		mainTopic := strings.TrimSuffix(record.Topic, cfg.RetrySuffix)
		sanitized := SanitizeForRepublish(record, mainTopic)
		// Preserve retry count so main consumer's MaxRetries logic continues to apply.
		recordCarrier{r: sanitized}.Set(HeaderRetryCount, recordCarrier{r: record}.Get(HeaderRetryCount))

		pubOpts := []PublishOpt{}
		if k := GetIdempotencyKey(sanitized); k != "" {
			pubOpts = append(pubOpts, WithIdempotencyKey(k))
		}
		for _, h := range sanitized.Headers {
			if h.Key == HeaderIdempotencyKey || h.Key == HeaderTraceParent || h.Key == HeaderTraceState {
				continue
			}
			pubOpts = append(pubOpts, WithHeader(h.Key, h.Value))
		}
		return producer.Publish(ctx, mainTopic, sanitized.Key, sanitized.Value, pubOpts...)
	})

	// Use a near-zero producer for the retry consumer's republish-on-error
	// path — but the retry consumer doesn't itself retry-to-retry; on failure
	// we simply don't commit and let the retry topic re-deliver.
	return Consume(ctx, client, handler, logger, ConsumeOptions{
		Consumer:  config.KafkaConsumerConfig{MaxRetries: math.MaxInt32, RetrySuffix: cfg.RetrySuffix, DLQSuffix: cfg.DLQSuffix},
		Producer:  producer,
		Heartbeat: heartbeat,
	})
}

func backoffDelay(base, max time.Duration, n int) time.Duration {
	if n < 0 {
		n = 0
	}
	d := base * (1 << uint(min(n, 16))) //#nosec G115 -- n is clamped to [0,16] above
	if d <= 0 || d > max {
		return max
	}
	return d
}
