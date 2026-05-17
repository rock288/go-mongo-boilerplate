package sqs

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// SQS MessageAttribute names. Reserved (consumed by the platform):
//
//	traceparent, tracestate     — W3C trace context
//	x-idempotency-key            — caller or auto-generated UUID
//
// Outbound:
//
//	x-retry-count                — bumped on each retry-queue republish
//	x-error-reason               — sanitized handler error (DLQ only)
const (
	HeaderTraceParent    = "traceparent"
	HeaderTraceState     = "tracestate"
	HeaderIdempotencyKey = "x-idempotency-key"
	HeaderRetryCount     = "x-retry-count"
	HeaderErrorReason    = "x-error-reason"

	// AWS SQS hard limit on MessageAttributes per message.
	MaxAttributes = 10

	// Reserved by the platform: traceparent, tracestate, x-idempotency-key.
	ReservedAttributes = 3

	// MaxCustomAttributes is what callers of Publish may add via WithAttribute.
	MaxCustomAttributes = MaxAttributes - ReservedAttributes

	maxErrorReasonLen = 256

	dataTypeString = "String"
)

// traceparent grammar: version-traceid-spanid-flags (hex).
var traceparentRe = regexp.MustCompile(`^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

// attrCarrier adapts a MessageAttributes map to TextMapCarrier so the OTel
// propagator can inject/extract trace context without knowing about SQS.
type attrCarrier struct {
	m map[string]types.MessageAttributeValue
}

func (c attrCarrier) Get(key string) string {
	for k, v := range c.m {
		if strings.EqualFold(k, key) && v.StringValue != nil {
			return *v.StringValue
		}
	}
	return ""
}

func (c attrCarrier) Set(key, value string) {
	c.m[key] = types.MessageAttributeValue{
		DataType:    aws.String(dataTypeString),
		StringValue: aws.String(value),
	}
}

func (c attrCarrier) Keys() []string {
	out := make([]string, 0, len(c.m))
	for k := range c.m {
		out = append(out, k)
	}
	return out
}

var _ propagation.TextMapCarrier = attrCarrier{}

// InjectTraceHeaders writes W3C trace context onto attrs from ctx.
func InjectTraceHeaders(ctx context.Context, attrs map[string]types.MessageAttributeValue) {
	otel.GetTextMapPropagator().Inject(ctx, attrCarrier{m: attrs})
}

// ExtractTraceContext returns a context with the trace propagated from msg attributes.
func ExtractTraceContext(ctx context.Context, msg *types.Message) context.Context {
	if msg == nil || msg.MessageAttributes == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, attrCarrier{m: msg.MessageAttributes})
}

// SetStringAttribute sets attrs[key] = String value.
func SetStringAttribute(attrs map[string]types.MessageAttributeValue, key, value string) {
	attrs[key] = types.MessageAttributeValue{
		DataType:    aws.String(dataTypeString),
		StringValue: aws.String(value),
	}
}

// GetStringAttribute reads attrs[key]'s string value, returning "" if absent
// or non-string. Lookup is case-insensitive (SQS preserves case but we are
// lenient on read).
func GetStringAttribute(attrs map[string]types.MessageAttributeValue, key string) string {
	if attrs == nil {
		return ""
	}
	for k, v := range attrs {
		if strings.EqualFold(k, key) && v.StringValue != nil {
			return *v.StringValue
		}
	}
	return ""
}

// SetIdempotencyKey sets or generates the idempotency attribute.
func SetIdempotencyKey(attrs map[string]types.MessageAttributeValue, key string) string {
	if key == "" {
		key = uuid.NewString()
	}
	SetStringAttribute(attrs, HeaderIdempotencyKey, key)
	return key
}

// GetIdempotencyKey reads the idempotency attribute from msg, "" if absent.
func GetIdempotencyKey(msg *types.Message) string {
	if msg == nil {
		return ""
	}
	return GetStringAttribute(msg.MessageAttributes, HeaderIdempotencyKey)
}

// RetryCount returns the parsed x-retry-count attribute (0 if absent or invalid).
func RetryCount(msg *types.Message) int {
	if msg == nil {
		return 0
	}
	v := GetStringAttribute(msg.MessageAttributes, HeaderRetryCount)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// IncRetryCount bumps x-retry-count by 1 on attrs and returns the new value.
func IncRetryCount(attrs map[string]types.MessageAttributeValue) int {
	v := GetStringAttribute(attrs, HeaderRetryCount)
	n := 0
	if v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			n = parsed
		}
	}
	n++
	SetStringAttribute(attrs, HeaderRetryCount, strconv.Itoa(n))
	return n
}

// SanitizeForRepublish returns a fresh attribute map suitable for republish.
// Attacker-controlled retry/error attributes are stripped; traceparent is
// preserved only when it matches the W3C grammar. Mirror of kafka.SanitizeForRepublish.
func SanitizeForRepublish(msg *types.Message) map[string]types.MessageAttributeValue {
	out := make(map[string]types.MessageAttributeValue, len(msg.MessageAttributes))
	for k, v := range msg.MessageAttributes {
		lk := strings.ToLower(k)
		switch lk {
		case HeaderRetryCount, HeaderErrorReason:
			continue
		case HeaderTraceParent:
			if v.StringValue == nil || !traceparentRe.MatchString(*v.StringValue) {
				continue
			}
		}
		out[k] = v
	}
	return out
}

// SanitizeError produces an attribute-safe representation of err: ASCII only,
// max 256 chars. Returns "unknown" for nil errors.
func SanitizeError(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := err.Error()
	var b strings.Builder
	b.Grow(len(msg))
	for _, r := range msg {
		if r > unicode.MaxASCII || r < 0x20 {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if len(out) > maxErrorReasonLen {
		out = out[:maxErrorReasonLen]
	}
	return out
}

// SetErrorReason writes a sanitized x-error-reason attribute.
func SetErrorReason(attrs map[string]types.MessageAttributeValue, err error) {
	SetStringAttribute(attrs, HeaderErrorReason, SanitizeError(err))
}
