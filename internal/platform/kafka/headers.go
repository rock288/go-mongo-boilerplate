package kafka

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const (
	HeaderTraceParent    = "traceparent"
	HeaderTraceState     = "tracestate"
	HeaderIdempotencyKey = "x-idempotency-key"
	HeaderRetryCount     = "x-retry-count"
	HeaderErrorReason    = "x-error-reason"

	maxErrorReasonLen = 256
)

// traceparent grammar: version-traceid-spanid-flags (hex)
// e.g. 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
var traceparentRe = regexp.MustCompile(`^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

// recordCarrier adapts kgo.Record.Headers to TextMapCarrier.
type recordCarrier struct{ r *kgo.Record }

func (c recordCarrier) Get(key string) string {
	for _, h := range c.r.Headers {
		if strings.EqualFold(h.Key, key) {
			return string(h.Value)
		}
	}
	return ""
}

func (c recordCarrier) Set(key, value string) {
	for i, h := range c.r.Headers {
		if strings.EqualFold(h.Key, key) {
			c.r.Headers[i].Value = []byte(value)
			return
		}
	}
	c.r.Headers = append(c.r.Headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

func (c recordCarrier) Keys() []string {
	out := make([]string, len(c.r.Headers))
	for i, h := range c.r.Headers {
		out[i] = h.Key
	}
	return out
}

// InjectTraceHeaders writes W3C trace context onto the record from ctx.
func InjectTraceHeaders(ctx context.Context, r *kgo.Record) {
	otel.GetTextMapPropagator().Inject(ctx, recordCarrier{r: r})
}

// ExtractTraceContext returns a context with the trace propagated from headers.
func ExtractTraceContext(ctx context.Context, r *kgo.Record) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, recordCarrier{r: r})
}

// SetIdempotencyKey sets or generates an idempotency key header.
// Returns the key set on the record.
func SetIdempotencyKey(r *kgo.Record, key string) string {
	if key == "" {
		key = uuid.NewString()
	}
	recordCarrier{r: r}.Set(HeaderIdempotencyKey, key)
	return key
}

// GetIdempotencyKey reads the idempotency key from headers (empty if absent).
func GetIdempotencyKey(r *kgo.Record) string {
	return recordCarrier{r: r}.Get(HeaderIdempotencyKey)
}

// RetryCount returns the current retry count from headers (0 if absent).
func RetryCount(r *kgo.Record) int {
	v := recordCarrier{r: r}.Get(HeaderRetryCount)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// IncRetryCount bumps x-retry-count by 1 and returns the new value.
func IncRetryCount(r *kgo.Record) int {
	n := RetryCount(r) + 1
	recordCarrier{r: r}.Set(HeaderRetryCount, strconv.Itoa(n))
	return n
}

// SanitizeForRepublish copies headers + value to a fresh record bound for
// topic. Attacker-controlled retry/error headers are stripped; traceparent
// is preserved only when it matches W3C grammar.
func SanitizeForRepublish(src *kgo.Record, topic string) *kgo.Record {
	dst := &kgo.Record{
		Topic: topic,
		Key:   append([]byte(nil), src.Key...),
		Value: append([]byte(nil), src.Value...),
	}
	for _, h := range src.Headers {
		k := strings.ToLower(h.Key)
		switch k {
		case HeaderRetryCount, HeaderErrorReason:
			continue
		case HeaderTraceParent:
			if !traceparentRe.MatchString(string(h.Value)) {
				continue
			}
		}
		dst.Headers = append(dst.Headers, kgo.RecordHeader{Key: h.Key, Value: append([]byte(nil), h.Value...)})
	}
	return dst
}

// SanitizeError produces a header-safe representation of err: ASCII only,
// max 256 chars, no struct dumps. Returns "unknown" for nil errors.
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

// SetErrorReason writes a sanitized x-error-reason header.
func SetErrorReason(r *kgo.Record, err error) {
	recordCarrier{r: r}.Set(HeaderErrorReason, SanitizeError(err))
}

// _ verifies recordCarrier satisfies the propagator interface at compile time.
var _ propagation.TextMapCarrier = recordCarrier{}

// ErrInvalidTraceparent is returned when a header fails W3C validation.
var ErrInvalidTraceparent = errors.New("kafka: invalid traceparent header")

// AssertValidTraceparent returns an error if v is non-empty but malformed.
func AssertValidTraceparent(v string) error {
	if v == "" {
		return nil
	}
	if !traceparentRe.MatchString(v) {
		return fmt.Errorf("%w: %q", ErrInvalidTraceparent, v)
	}
	return nil
}
