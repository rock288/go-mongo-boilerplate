package kafka

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type fakeWriter struct {
	mu      sync.Mutex
	records []*kgo.Record
	err     error
}

func (f *fakeWriter) ProduceSync(_ context.Context, rs ...*kgo.Record) kgo.ProduceResults {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, rs...)
	out := make(kgo.ProduceResults, len(rs))
	for i, r := range rs {
		out[i] = kgo.ProduceResult{Record: r, Err: f.err}
	}
	return out
}

func (f *fakeWriter) lastRecord() *kgo.Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.records) == 0 {
		return nil
	}
	return f.records[len(f.records)-1]
}

func newTestProducer(w kafkaWriter) Producer {
	return &producer{client: w}
}

func TestProducer_Publish_Success(t *testing.T) {
	t.Parallel()

	w := &fakeWriter{}
	p := newTestProducer(w)
	err := p.Publish(context.Background(), "topic.test", []byte("k"), []byte("v"))
	require.NoError(t, err)

	rec := w.lastRecord()
	require.NotNil(t, rec)
	assert.Equal(t, "topic.test", rec.Topic)
	assert.Equal(t, []byte("k"), rec.Key)
	assert.Equal(t, []byte("v"), rec.Value)
}

func TestProducer_Publish_GeneratesIdempotencyKey_WhenAbsent(t *testing.T) {
	t.Parallel()

	w := &fakeWriter{}
	p := newTestProducer(w)
	require.NoError(t, p.Publish(context.Background(), "t", nil, []byte("v")))

	got := GetIdempotencyKey(w.lastRecord())
	assert.NotEmpty(t, got, "idempotency key should be auto-generated")
}

func TestProducer_Publish_PreservesIdempotencyKey_WhenProvided(t *testing.T) {
	t.Parallel()

	w := &fakeWriter{}
	p := newTestProducer(w)
	require.NoError(t, p.Publish(context.Background(), "t", nil, []byte("v"),
		WithIdempotencyKey("my-key-123")))

	assert.Equal(t, "my-key-123", GetIdempotencyKey(w.lastRecord()))
}

func TestProducer_Publish_InjectsTraceparent(t *testing.T) {
	t.Parallel()

	// Set up a real tracer + propagator so traceparent gets injected.
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prevTP) })

	w := &fakeWriter{}
	p := newTestProducer(w)
	require.NoError(t, p.Publish(context.Background(), "t", nil, []byte("v")))

	rec := w.lastRecord()
	var foundTraceparent bool
	for _, h := range rec.Headers {
		if h.Key == HeaderTraceParent {
			foundTraceparent = true
			assert.NoError(t, AssertValidTraceparent(string(h.Value)),
				"injected traceparent must satisfy W3C grammar")
		}
	}
	assert.True(t, foundTraceparent, "traceparent should be injected")
}

func TestProducer_Publish_AppendsExtraHeaders(t *testing.T) {
	t.Parallel()

	w := &fakeWriter{}
	p := newTestProducer(w)
	require.NoError(t, p.Publish(context.Background(), "t", nil, nil,
		WithHeader("x-custom", []byte("hello"))))

	rec := w.lastRecord()
	var found bool
	for _, h := range rec.Headers {
		if h.Key == "x-custom" && string(h.Value) == "hello" {
			found = true
		}
	}
	assert.True(t, found, "custom header should be appended")
}

func TestProducer_Publish_WriterError_Wrapped(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("broker unavailable")
	w := &fakeWriter{err: wantErr}
	p := newTestProducer(w)

	err := p.Publish(context.Background(), "topic.x", nil, []byte("v"))
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Contains(t, err.Error(), "topic.x")
}

func TestProducer_Publish_ForceSampleOption(t *testing.T) {
	t.Parallel()

	// Sanity: ForceSample must not break the happy path.
	w := &fakeWriter{}
	p := newTestProducer(w)
	require.NoError(t, p.Publish(context.Background(), "t", nil, nil, ForceSample()))
}
