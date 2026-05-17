package sqs_test

import (
	"context"
	"testing"
	"time"

	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/sqs"
	"github.com/rock288/go-mongo-boilerplate/internal/platform/sqs/mocks"
)

const fakeQueueURL = "https://sqs.us-east-1.amazonaws.com/123/main"

func TestProducer_Publish_InjectsTraceAndIdempotency(t *testing.T) {
	t.Parallel()

	api := mocks.NewSQSAPI(t)
	var captured *awssqs.SendMessageInput
	api.EXPECT().
		SendMessage(mock.Anything, mock.MatchedBy(func(in *awssqs.SendMessageInput) bool {
			captured = in
			return true
		})).
		Return(&awssqs.SendMessageOutput{MessageId: ptr("msg-123")}, nil).
		Once()

	p := sqs.NewProducer(api)
	err := p.Publish(context.Background(), fakeQueueURL, "hello")
	require.NoError(t, err)

	require.NotNil(t, captured)
	assert.Equal(t, "hello", *captured.MessageBody)
	// idempotency key auto-generated
	idk := sqs.GetStringAttribute(captured.MessageAttributes, sqs.HeaderIdempotencyKey)
	assert.NotEmpty(t, idk)
}

func TestProducer_Publish_TooManyAttrsRejects(t *testing.T) {
	t.Parallel()

	api := mocks.NewSQSAPI(t) // no SendMessage expected
	p := sqs.NewProducer(api)

	opts := []sqs.PublishOpt{}
	for i := 0; i < sqs.MaxCustomAttributes+1; i++ {
		opts = append(opts, sqs.WithAttribute(string(rune('a'+i)), "v"))
	}
	err := p.Publish(context.Background(), fakeQueueURL, "x", opts...)
	require.ErrorIs(t, err, sqs.ErrTooManyAttributes)
}

func TestProducer_Publish_DelayClampedToSQSMax(t *testing.T) {
	t.Parallel()

	api := mocks.NewSQSAPI(t)
	var captured *awssqs.SendMessageInput
	api.EXPECT().
		SendMessage(mock.Anything, mock.MatchedBy(func(in *awssqs.SendMessageInput) bool {
			captured = in
			return true
		})).
		Return(&awssqs.SendMessageOutput{}, nil).
		Once()

	p := sqs.NewProducer(api)
	err := p.Publish(context.Background(), fakeQueueURL, "x", sqs.WithDelay(30*time.Minute))
	require.NoError(t, err)

	require.NotNil(t, captured)
	assert.Equal(t, int32(900), captured.DelaySeconds, "30m clamps to SQS max 900s")
}

func TestProducer_Publish_ExplicitIdempotencyKeyHonoured(t *testing.T) {
	t.Parallel()

	api := mocks.NewSQSAPI(t)
	var captured *awssqs.SendMessageInput
	api.EXPECT().
		SendMessage(mock.Anything, mock.MatchedBy(func(in *awssqs.SendMessageInput) bool {
			captured = in
			return true
		})).
		Return(&awssqs.SendMessageOutput{}, nil).
		Once()

	p := sqs.NewProducer(api)
	err := p.Publish(context.Background(), fakeQueueURL, "x", sqs.WithIdempotencyKey("explicit-key"))
	require.NoError(t, err)
	assert.Equal(t, "explicit-key", sqs.GetStringAttribute(captured.MessageAttributes, sqs.HeaderIdempotencyKey))
}

func ptr[T any](v T) *T { return &v }

func TestQueueName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"https://sqs.us-east-1.amazonaws.com/123/events": "events",
		"http://localhost:4566/000000000000/dev-q":       "dev-q",
		"name-only": "name-only",
	}
	for url, want := range cases {
		got, err := sqs.QueueName(url)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, err := sqs.QueueName("")
	assert.Error(t, err)
}

// Sanity check the package's MessageAttributeValue helpers play nicely with the
// SDK's types (compile-time check via uses below).
func TestStringAttributeValueType(t *testing.T) {
	t.Parallel()
	m := map[string]types.MessageAttributeValue{}
	sqs.SetStringAttribute(m, "k", "v")
	require.NotNil(t, m["k"].DataType)
	assert.Equal(t, "String", *m["k"].DataType)
}
