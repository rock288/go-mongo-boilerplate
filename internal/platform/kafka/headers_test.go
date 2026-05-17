package kafka

import (
	"errors"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestSanitizeForRepublish_StripsAttackerHeaders(t *testing.T) {
	src := &kgo.Record{
		Topic: "events",
		Value: []byte("hi"),
		Headers: []kgo.RecordHeader{
			{Key: HeaderRetryCount, Value: []byte("999999")},
			{Key: HeaderErrorReason, Value: []byte("attacker-supplied")},
			{Key: "x-business", Value: []byte("keep-me")},
		},
	}
	dst := SanitizeForRepublish(src, "events.retry")
	for _, h := range dst.Headers {
		if h.Key == HeaderRetryCount || h.Key == HeaderErrorReason {
			t.Fatalf("expected %s to be stripped, got: %s", h.Key, h.Value)
		}
	}
	found := false
	for _, h := range dst.Headers {
		if h.Key == "x-business" && string(h.Value) == "keep-me" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected legitimate header preserved")
	}
	if dst.Topic != "events.retry" {
		t.Fatalf("expected topic events.retry, got %s", dst.Topic)
	}
}

func TestSanitizeForRepublish_DropsMalformedTraceparent(t *testing.T) {
	src := &kgo.Record{
		Headers: []kgo.RecordHeader{
			{Key: HeaderTraceParent, Value: []byte("not-a-valid-traceparent")},
		},
	}
	dst := SanitizeForRepublish(src, "events.retry")
	for _, h := range dst.Headers {
		if strings.EqualFold(h.Key, HeaderTraceParent) {
			t.Fatalf("expected malformed traceparent dropped, got %s", h.Value)
		}
	}
}

func TestSanitizeForRepublish_KeepsValidTraceparent(t *testing.T) {
	valid := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	src := &kgo.Record{
		Headers: []kgo.RecordHeader{
			{Key: HeaderTraceParent, Value: []byte(valid)},
		},
	}
	dst := SanitizeForRepublish(src, "events.retry")
	found := false
	for _, h := range dst.Headers {
		if h.Key == HeaderTraceParent && string(h.Value) == valid {
			found = true
		}
	}
	if !found {
		t.Fatal("expected valid traceparent preserved")
	}
}

func TestSanitizeError_TruncatesAndStripsNonASCII(t *testing.T) {
	long := strings.Repeat("a", 500) + "🚀"
	out := SanitizeError(errors.New(long))
	if len(out) > maxErrorReasonLen {
		t.Fatalf("expected ≤%d chars, got %d", maxErrorReasonLen, len(out))
	}
	if strings.Contains(out, "🚀") {
		t.Fatal("expected non-ASCII stripped")
	}
}

func TestSanitizeError_NilErrorReturnsUnknown(t *testing.T) {
	if got := SanitizeError(nil); got != "unknown" {
		t.Fatalf("expected 'unknown', got %q", got)
	}
}

func TestRetryCount_NegativeAndJunkResetToZero(t *testing.T) {
	r := &kgo.Record{Headers: []kgo.RecordHeader{
		{Key: HeaderRetryCount, Value: []byte("-5")},
	}}
	if RetryCount(r) != 0 {
		t.Fatal("expected negative to clamp to 0")
	}
	r.Headers[0].Value = []byte("junk")
	if RetryCount(r) != 0 {
		t.Fatal("expected non-numeric to clamp to 0")
	}
}

func TestIncRetryCount_StartsAtOne(t *testing.T) {
	r := &kgo.Record{}
	if n := IncRetryCount(r); n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
	if n := IncRetryCount(r); n != 2 {
		t.Fatalf("expected 2, got %d", n)
	}
}

func TestRecordCarrier_Keys_ReturnsAllHeaderKeys(t *testing.T) {
	r := &kgo.Record{Headers: []kgo.RecordHeader{
		{Key: "a"}, {Key: "b"}, {Key: "c"},
	}}
	got := recordCarrier{r: r}.Keys()
	if len(got) != 3 {
		t.Fatalf("expected 3 keys, got %v", got)
	}
	want := map[string]bool{"a": true, "b": true, "c": true}
	for _, k := range got {
		if !want[k] {
			t.Errorf("unexpected key %q", k)
		}
	}
}

func TestSetIdempotencyKey_GeneratesWhenEmpty(t *testing.T) {
	r := &kgo.Record{}
	got := SetIdempotencyKey(r, "")
	if got == "" {
		t.Fatal("expected auto-generated UUID")
	}
	if GetIdempotencyKey(r) != got {
		t.Fatal("idempotency key not persisted on record")
	}
}

func TestSetIdempotencyKey_PreservesExplicit(t *testing.T) {
	r := &kgo.Record{}
	got := SetIdempotencyKey(r, "explicit-key")
	if got != "explicit-key" {
		t.Fatalf("expected explicit-key, got %q", got)
	}
}

func TestAssertValidTraceparent(t *testing.T) {
	if err := AssertValidTraceparent(""); err != nil {
		t.Fatal("empty must be allowed")
	}
	valid := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	if err := AssertValidTraceparent(valid); err != nil {
		t.Fatalf("valid traceparent rejected: %v", err)
	}
	if err := AssertValidTraceparent("garbage"); err == nil {
		t.Fatal("expected error for malformed traceparent")
	}
}
