package sqs

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const producerTracer = "github.com/rock288/go-mongo-boilerplate/internal/platform/sqs.producer"

// Producer is the boilerplate-facing contract for publishing SQS messages.
// Auto-injects traceparent + x-idempotency-key MessageAttributes.
type Producer interface {
	Publish(ctx context.Context, queueURL, body string, opts ...PublishOpt) error
}

// PublishOpt is a functional option for Publish.
type PublishOpt func(*publishConfig)

type publishConfig struct {
	idempotencyKey string
	extraAttrs     map[string]string
	delay          time.Duration
	forceSample    bool
}

// WithIdempotencyKey overrides the auto-generated UUID.
func WithIdempotencyKey(k string) PublishOpt {
	return func(c *publishConfig) { c.idempotencyKey = k }
}

// WithAttribute adds a custom String MessageAttribute. Counts against
// MaxCustomAttributes (7) — Publish rejects with ErrTooManyAttributes if
// the caller exceeds the budget.
func WithAttribute(key, value string) PublishOpt {
	return func(c *publishConfig) {
		if c.extraAttrs == nil {
			c.extraAttrs = map[string]string{}
		}
		c.extraAttrs[key] = value
	}
}

// WithDelay sets MessageDelaySeconds. Clamped to [0, SQSMaxDelay] silently.
func WithDelay(d time.Duration) PublishOpt {
	return func(c *publishConfig) { c.delay = d }
}

// ForceSample creates a new root span (always sampled) so retry/DLQ traffic
// stays visible even when the parent trace was sampled out. Mirror of
// kafka.ForceSample.
func ForceSample() PublishOpt {
	return func(c *publishConfig) { c.forceSample = true }
}

type producer struct {
	client SQSAPI
}

// NewProducer returns a Producer backed by the SQS client.
func NewProducer(client SQSAPI) Producer { return &producer{client: client} }

func (p *producer) Publish(ctx context.Context, queueURL, body string, opts ...PublishOpt) error {
	cfg := &publishConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if len(cfg.extraAttrs) > MaxCustomAttributes {
		return fmt.Errorf("%w: got %d, max %d", ErrTooManyAttributes, len(cfg.extraAttrs), MaxCustomAttributes)
	}

	attrs := make(map[string]types.MessageAttributeValue, len(cfg.extraAttrs)+ReservedAttributes)
	for k, v := range cfg.extraAttrs {
		SetStringAttribute(attrs, k, v)
	}
	SetIdempotencyKey(attrs, cfg.idempotencyKey)

	qname, _ := QueueName(queueURL)
	tracer := otel.Tracer(producerTracer)
	startOpts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "aws_sqs"),
			attribute.String("messaging.operation", "publish"),
			attribute.String("messaging.destination.name", qname),
			attribute.String("messaging.destination.kind", "queue"),
		),
	}
	if cfg.forceSample {
		startOpts = append(startOpts, trace.WithNewRoot())
	}
	spanCtx, span := tracer.Start(ctx, "sqs.publish "+qname, startOpts...)
	defer span.End()

	InjectTraceHeaders(spanCtx, attrs)

	in := &awssqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(body),
		MessageAttributes: attrs,
	}
	if cfg.delay > 0 {
		d := cfg.delay
		if d > SQSMaxDelay {
			d = SQSMaxDelay
		}
		in.DelaySeconds = int32(d / time.Second) //#nosec G115 -- d clamped to SQSMaxDelay (900s) above
	}

	out, err := p.client.SendMessage(spanCtx, in)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("sqs publish %s: %w", qname, err)
	}
	if out != nil && out.MessageId != nil {
		span.SetAttributes(attribute.String("messaging.message.id", *out.MessageId))
	}
	return nil
}
