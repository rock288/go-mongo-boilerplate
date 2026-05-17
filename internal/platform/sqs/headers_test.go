package sqs

import (
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
)

func attrs(kv ...string) map[string]types.MessageAttributeValue {
	m := map[string]types.MessageAttributeValue{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(kv[i+1]),
		}
	}
	return m
}

func TestSetGetStringAttribute_Roundtrip(t *testing.T) {
	t.Parallel()
	m := map[string]types.MessageAttributeValue{}
	SetStringAttribute(m, "k", "v")
	assert.Equal(t, "v", GetStringAttribute(m, "k"))
	assert.Equal(t, "v", GetStringAttribute(m, "K"), "case-insensitive read")
	assert.Empty(t, GetStringAttribute(m, "missing"))
	assert.Empty(t, GetStringAttribute(nil, "k"))
}

func TestIdempotencyKey_GeneratesOrAccepts(t *testing.T) {
	t.Parallel()
	m := map[string]types.MessageAttributeValue{}
	k1 := SetIdempotencyKey(m, "")
	assert.NotEmpty(t, k1)
	assert.Equal(t, k1, GetStringAttribute(m, HeaderIdempotencyKey))

	k2 := SetIdempotencyKey(m, "explicit")
	assert.Equal(t, "explicit", k2)
}

func TestRetryCount_ParseAndIncrement(t *testing.T) {
	t.Parallel()

	msg := &types.Message{MessageAttributes: attrs()}
	assert.Zero(t, RetryCount(msg))

	msg.MessageAttributes[HeaderRetryCount] = types.MessageAttributeValue{
		DataType: aws.String("String"), StringValue: aws.String("3"),
	}
	assert.Equal(t, 3, RetryCount(msg))

	// Garbage and negative → 0
	msg.MessageAttributes[HeaderRetryCount] = types.MessageAttributeValue{
		DataType: aws.String("String"), StringValue: aws.String("not-a-number"),
	}
	assert.Zero(t, RetryCount(msg))

	// Inc on zero attrs starts at 1
	a := map[string]types.MessageAttributeValue{}
	assert.Equal(t, 1, IncRetryCount(a))
	assert.Equal(t, 2, IncRetryCount(a))
	assert.Equal(t, "2", GetStringAttribute(a, HeaderRetryCount))
}

func TestSanitizeForRepublish(t *testing.T) {
	t.Parallel()

	in := &types.Message{
		MessageAttributes: attrs(
			HeaderTraceParent, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			HeaderRetryCount, "5", // attacker-controlled, must be stripped
			HeaderErrorReason, "oops", // attacker-controlled, must be stripped
			HeaderIdempotencyKey, "abc",
			"x-custom", "keep-me",
		),
	}
	out := SanitizeForRepublish(in)
	assert.NotContains(t, out, HeaderRetryCount)
	assert.NotContains(t, out, HeaderErrorReason)
	assert.Equal(t, "abc", GetStringAttribute(out, HeaderIdempotencyKey))
	assert.Equal(t, "keep-me", GetStringAttribute(out, "x-custom"))
	assert.NotEmpty(t, GetStringAttribute(out, HeaderTraceParent))
}

func TestSanitizeForRepublish_DropsMalformedTraceparent(t *testing.T) {
	t.Parallel()
	in := &types.Message{
		MessageAttributes: attrs(HeaderTraceParent, "garbage"),
	}
	out := SanitizeForRepublish(in)
	assert.NotContains(t, out, HeaderTraceParent)
}

func TestSanitizeError(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unknown", SanitizeError(nil))
	assert.Equal(t, "boom", SanitizeError(errors.New("boom")))

	// Non-ASCII becomes '?'
	got := SanitizeError(errors.New("héllo"))
	assert.True(t, strings.Contains(got, "?"))

	// Long messages truncated at 256
	long := strings.Repeat("a", 500)
	got = SanitizeError(errors.New(long))
	assert.Len(t, got, maxErrorReasonLen)
}
