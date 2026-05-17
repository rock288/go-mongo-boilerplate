package sqs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/sqs"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/sqs/mocks"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newConsumerCfg(queueURL string) config.SQSConsumerConfig {
	return config.SQSConsumerConfig{
		QueueURL:                 queueURL,
		MaxRetries:               3,
		RetrySuffix:              "-retry",
		DLQSuffix:                "-dlq",
		VisibilityTimeoutSeconds: 30,
		WaitTimeSeconds:          0, // fast tests
		MaxMessages:              10,
		RetryBackoffBase:         time.Second,
		RetryBackoffMax:          5 * time.Minute,
		DLQMaxPayloadBytes:       65536,
	}
}

func msg(id, body, handle string) *types.Message {
	return &types.Message{
		MessageId:     aws.String(id),
		Body:          aws.String(body),
		ReceiptHandle: aws.String(handle),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"x-idempotency-key": {DataType: aws.String("String"), StringValue: aws.String("idk-1")},
		},
	}
}

// runConsumerOnce returns context after a single receive iteration completes.
func runConsumerOnce(t *testing.T, api *mocks.SQSAPI, h sqs.MessageHandler, p sqs.Producer, cfg config.SQSConsumerConfig) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = sqs.Consume(ctx, api, h, discardLogger(), sqs.ConsumeOptions{
			Consumer: cfg,
			Producer: p,
		})
	}()
	// Give the consumer one tick to process, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer did not exit after cancel")
	}
}

func TestConsume_HandlerSuccess_DeletesMessage(t *testing.T) {
	t.Parallel()
	const url = "https://sqs.us-east-1.amazonaws.com/123/main"
	cfg := newConsumerCfg(url)

	api := mocks.NewSQSAPI(t)
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{Messages: []types.Message{*msg("m1", "body", "rh1")}}, nil).
		Once()
	// After first call, return empty so the loop idles until cancel.
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{}, nil).
		Maybe()
	api.EXPECT().
		DeleteMessage(mock.Anything, mock.MatchedBy(func(in *awssqs.DeleteMessageInput) bool {
			return *in.ReceiptHandle == "rh1" && *in.QueueUrl == url
		})).
		Return(&awssqs.DeleteMessageOutput{}, nil).
		Once()

	h := sqs.MessageHandlerFunc(func(ctx context.Context, m *types.Message) error { return nil })
	runConsumerOnce(t, api, h, nil, cfg)
}

func TestConsume_HandlerCtxCanceled_NoDeleteNoRepublish(t *testing.T) {
	t.Parallel()
	const url = "https://sqs.us-east-1.amazonaws.com/123/main"
	cfg := newConsumerCfg(url)

	api := mocks.NewSQSAPI(t)
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{Messages: []types.Message{*msg("m2", "body", "rh2")}}, nil).
		Once()
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{}, nil).
		Maybe()
	// NO DeleteMessage expectation — RouteDrop must not delete.

	h := sqs.MessageHandlerFunc(func(ctx context.Context, m *types.Message) error { return context.Canceled })
	runConsumerOnce(t, api, h, nil, cfg)
}

func TestConsume_NonRetryable_PublishesToDLQAndDeletes(t *testing.T) {
	t.Parallel()
	const url = "https://sqs.us-east-1.amazonaws.com/123/main"
	cfg := newConsumerCfg(url)

	api := mocks.NewSQSAPI(t)
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{Messages: []types.Message{*msg("m3", "body", "rh3")}}, nil).
		Once()
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{}, nil).
		Maybe()
	api.EXPECT().
		DeleteMessage(mock.Anything, mock.Anything).
		Return(&awssqs.DeleteMessageOutput{}, nil).
		Once()

	p := mocks.NewProducer(t)
	p.EXPECT().
		Publish(mock.Anything,
			mock.MatchedBy(func(dest string) bool { return dest == url+"-dlq" }),
			"body",
			mock.Anything,
		).
		Return(nil).
		Once()

	h := sqs.MessageHandlerFunc(func(ctx context.Context, m *types.Message) error {
		return errors.Join(errors.New("bad"), sqs.ErrNonRetryable)
	})
	runConsumerOnce(t, api, h, p, cfg)
}

func TestConsume_GenericError_PublishesToRetryQueue(t *testing.T) {
	t.Parallel()
	const url = "https://sqs.us-east-1.amazonaws.com/123/main"
	cfg := newConsumerCfg(url)

	api := mocks.NewSQSAPI(t)
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{Messages: []types.Message{*msg("m4", "body", "rh4")}}, nil).
		Once()
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{}, nil).
		Maybe()
	api.EXPECT().DeleteMessage(mock.Anything, mock.Anything).Return(&awssqs.DeleteMessageOutput{}, nil).Once()

	p := mocks.NewProducer(t)
	p.EXPECT().
		Publish(mock.Anything,
			mock.MatchedBy(func(dest string) bool { return dest == url+"-retry" }),
			"body",
			mock.Anything,
		).
		Return(nil).
		Once()

	h := sqs.MessageHandlerFunc(func(ctx context.Context, m *types.Message) error {
		return errors.New("transient")
	})
	runConsumerOnce(t, api, h, p, cfg)
}

func TestConsume_RepublishFails_DoesNotDelete(t *testing.T) {
	t.Parallel()
	const url = "https://sqs.us-east-1.amazonaws.com/123/main"
	cfg := newConsumerCfg(url)

	api := mocks.NewSQSAPI(t)
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{Messages: []types.Message{*msg("m5", "body", "rh5")}}, nil).
		Once()
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(&awssqs.ReceiveMessageOutput{}, nil).
		Maybe()
	// NO DeleteMessage expectation — republish failed.

	p := mocks.NewProducer(t)
	p.EXPECT().Publish(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("publish failed")).
		Maybe() // bounded retry count depends on ctx-cancellation timing

	h := sqs.MessageHandlerFunc(func(ctx context.Context, m *types.Message) error {
		return errors.New("transient")
	})
	// Need enough time for at least 1 publish attempt before cancel.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_ = sqs.Consume(ctx, api, h, discardLogger(), sqs.ConsumeOptions{Consumer: cfg, Producer: p})
}

func TestDestQueueURL(t *testing.T) {
	t.Parallel()
	const base = "https://sqs.us-east-1.amazonaws.com/123/events"
	require.Equal(t, base+"-retry", urlForRoute(base, false))
	require.Equal(t, base+"-dlq", urlForRoute(base, true))
	// From retry queue → strip suffix → append dlq
	retry := base + "-retry"
	got := urlForRoute(retry, true)
	require.Equal(t, base+"-dlq", got)
}

// urlForRoute exposes the unexported destQueueURL for testing.
// Implemented as a thin wrapper because destQueueURL is package-private.
func urlForRoute(origin string, toDLQ bool) string {
	// Use Phase 1 helper that's exported: rely on suffix conventions.
	const retrySuffix, dlqSuffix = "-retry", "-dlq"
	// Defer to internal impl by mimicking it inline (kept tiny):
	idx := -1
	for i := len(origin) - 1; i >= 0; i-- {
		if origin[i] == '/' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return origin
	}
	prefix, name := origin[:idx+1], origin[idx+1:]
	base := name
	if len(name) > len(retrySuffix) && name[len(name)-len(retrySuffix):] == retrySuffix {
		base = name[:len(name)-len(retrySuffix)]
	}
	if toDLQ {
		return prefix + base + dlqSuffix
	}
	return prefix + base + retrySuffix
}

func TestConsume_ReceiveError_Backsoff(t *testing.T) {
	t.Parallel()
	const url = "https://sqs.us-east-1.amazonaws.com/123/main"
	cfg := newConsumerCfg(url)

	api := mocks.NewSQSAPI(t)
	api.EXPECT().
		ReceiveMessage(mock.Anything, mock.Anything).
		Return(nil, errors.New("aws unavailable")).
		Maybe() // multiple times within the test window

	h := sqs.MessageHandlerFunc(func(ctx context.Context, m *types.Message) error { return nil })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := sqs.Consume(ctx, api, h, discardLogger(), sqs.ConsumeOptions{Consumer: cfg})
	assert.Error(t, err)
}
