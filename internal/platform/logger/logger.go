package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// New builds a *slog.Logger from logger config.
// JSON handler is used unless format=="text" (dev-friendly).
// Auto-injects trace_id/span_id when records are emitted within a span context.
func New(cfg config.LoggerConfig) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: strings.EqualFold(cfg.Format, "text"),
	}

	var base slog.Handler
	if strings.EqualFold(cfg.Format, "text") {
		base = slog.NewTextHandler(os.Stdout, opts)
	} else {
		base = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(&traceHandler{Handler: base})
	slog.SetDefault(logger)
	return logger
}

// traceHandler wraps a slog.Handler and decorates records with trace_id /
// span_id when called inside an active OTel span. This avoids pulling in
// otelslog (an OTLP log exporter) and keeps logs flowing to stdout while
// preserving trace correlation in SigNoz when stdout is scraped.
type traceHandler struct {
	slog.Handler
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		sc := span.SpanContext()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithGroup(name)}
}
