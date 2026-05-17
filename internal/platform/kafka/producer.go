package kafka

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const producerTracer = "github.com/rock288/go-mongo-boilerplate/internal/platform/kafka.producer"

// Producer is the boilerplate-facing contract for publishing messages.
// Auto-injects traceparent + x-idempotency-key headers.
type Producer interface {
	Publish(ctx context.Context, topic string, key, value []byte, opts ...PublishOpt) error
}

// PublishOpt is a functional option for Publish.
type PublishOpt func(*publishConfig)

type publishConfig struct {
	idempotencyKey string
	extraHeaders   []kgo.RecordHeader
	forceSample    bool
}

// WithIdempotencyKey overrides the auto-generated UUID.
func WithIdempotencyKey(k string) PublishOpt {
	return func(c *publishConfig) { c.idempotencyKey = k }
}

// WithHeader appends a header.
func WithHeader(k string, v []byte) PublishOpt {
	return func(c *publishConfig) {
		c.extraHeaders = append(c.extraHeaders, kgo.RecordHeader{Key: k, Value: v})
	}
}

// ForceSample creates a new root span (always-sampled) for this publish so
// retry/DLQ traffic remains observable when parent context was sampled out.
func ForceSample() PublishOpt {
	return func(c *publishConfig) { c.forceSample = true }
}

// kafkaWriter is the subset of *kgo.Client that the producer needs.
// Exists to make Publish testable without a live broker.
type kafkaWriter interface {
	ProduceSync(ctx context.Context, rs ...*kgo.Record) kgo.ProduceResults
}

type producer struct {
	client kafkaWriter
}

// NewProducerWrapper wraps an existing kgo.Client.
func NewProducerWrapper(client *kgo.Client) Producer {
	return &producer{client: client}
}

func (p *producer) Publish(ctx context.Context, topic string, key, value []byte, opts ...PublishOpt) error {
	cfg := &publishConfig{}
	for _, o := range opts {
		o(cfg)
	}

	rec := &kgo.Record{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: append([]kgo.RecordHeader{}, cfg.extraHeaders...),
	}
	SetIdempotencyKey(rec, cfg.idempotencyKey)

	tracer := otel.Tracer(producerTracer)
	startOpts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination", topic),
		),
	}
	if cfg.forceSample {
		startOpts = append(startOpts, trace.WithNewRoot())
	}
	spanCtx, span := tracer.Start(ctx, "kafka.publish "+topic, startOpts...)
	defer span.End()

	InjectTraceHeaders(spanCtx, rec)

	res := p.client.ProduceSync(spanCtx, rec)
	if err := res.FirstErr(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("kafka publish %s: %w", topic, err)
	}
	return nil
}
