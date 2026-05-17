package sqs_test

import (
	"context"
	"testing"

	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/sqs"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/sqs/mocks"
)

// Verifies producer injects a W3C-grammar-valid traceparent that the consumer
// can extract back into ctx.
func TestTracePropagation_Roundtrip(t *testing.T) {
	// Cannot t.Parallel — touches global TracerProvider + Propagator.

	originalTP := otel.GetTracerProvider()
	originalProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(originalTP)
		otel.SetTextMapPropagator(originalProp)
	})

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	// Producer captures the SendMessageInput so we can read the injected attrs.
	api := mocks.NewSQSAPI(t)
	var captured *awssqs.SendMessageInput
	api.EXPECT().
		SendMessage(mock.Anything, mock.MatchedBy(func(in *awssqs.SendMessageInput) bool {
			captured = in
			return true
		})).
		Return(&awssqs.SendMessageOutput{}, nil).
		Once()

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "publish-flow")
	defer span.End()

	p := sqs.NewProducer(api)
	require.NoError(t, p.Publish(ctx, "https://sqs.us-east-1.amazonaws.com/123/q", "x"))

	traceparent := sqs.GetStringAttribute(captured.MessageAttributes, sqs.HeaderTraceParent)
	require.NotEmpty(t, traceparent, "traceparent must be injected")
	assert.Regexp(t, `^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`, traceparent)

	// Consumer side: build a fake message with those attrs, extract → context
	// should carry a valid span context with the same trace ID.
	msg := &types.Message{MessageAttributes: captured.MessageAttributes}
	extractedCtx := sqs.ExtractTraceContext(context.Background(), msg)
	got := trace.SpanContextFromContext(extractedCtx)
	require.True(t, got.IsValid(), "extracted span context must be valid")

	want := trace.SpanContextFromContext(ctx)
	assert.Equal(t, want.TraceID(), got.TraceID(), "trace ID preserved end-to-end")
}
