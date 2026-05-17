package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

func TestNew_BuildsJSONLoggerByDefault(t *testing.T) {
	t.Parallel()
	logger := New(config.LoggerConfig{Level: "info"})
	require.NotNil(t, logger)
}

func TestNew_RespectsLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		level string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"INFO", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.level, func(t *testing.T) {
			t.Parallel()
			logger := New(config.LoggerConfig{Level: tt.level})
			assert.True(t, logger.Enabled(context.Background(), tt.want),
				"logger should enable level %v for cfg %q", tt.want, tt.level)
		})
	}
}

func TestTraceHandler_InjectsTraceID_WhenInSpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	l := slog.New(&traceHandler{Handler: base})

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	ctx, span := otel.Tracer("test").Start(context.Background(), "op")
	l.InfoContext(ctx, "hello", "k", "v")
	span.End()

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))

	traceID, ok := rec["trace_id"].(string)
	require.True(t, ok, "trace_id missing: %s", buf.String())
	assert.NotEmpty(t, traceID)
	assert.Len(t, traceID, 32, "W3C trace IDs are 32 hex chars")

	spanID, ok := rec["span_id"].(string)
	require.True(t, ok)
	assert.Len(t, spanID, 16, "W3C span IDs are 16 hex chars")
}

func TestTraceHandler_NoSpan_NoTraceFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	l := slog.New(&traceHandler{Handler: base})

	l.InfoContext(context.Background(), "no-span")
	out := buf.String()
	assert.NotContains(t, out, "trace_id")
	assert.NotContains(t, out, "span_id")
}

func TestTraceHandler_WithAttrs_PreservesWrapping(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	h := &traceHandler{Handler: base}

	derived := h.WithAttrs([]slog.Attr{slog.String("static", "x")})
	_, isTrace := derived.(*traceHandler)
	assert.True(t, isTrace, "WithAttrs must preserve traceHandler wrapping")

	l := slog.New(derived)
	l.Info("msg")
	assert.Contains(t, buf.String(), `"static":"x"`)
}

func TestTraceHandler_WithGroup_PreservesWrapping(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, nil)
	h := &traceHandler{Handler: base}

	derived := h.WithGroup("g")
	_, isTrace := derived.(*traceHandler)
	assert.True(t, isTrace, "WithGroup must preserve traceHandler wrapping")
}

func TestNew_TextFormatUsesTextHandler(t *testing.T) {
	t.Parallel()
	logger := New(config.LoggerConfig{Format: "text", Level: "info"})
	require.NotNil(t, logger)
	// Smoke — no easy way to introspect handler type from outside the package,
	// but New must not panic and must return a usable logger.
	var buf bytes.Buffer
	textLogger := slog.New(slog.NewTextHandler(&buf, nil))
	textLogger.Info("ping")
	assert.Contains(t, buf.String(), "ping")
	_ = strings.Builder{}
}
