package database

import (
	"context"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	mongoTracerName = "github.com/rock288/go-mongo-boilerplate/internal/platform/database"
	mongoSpanTTL    = 2 * time.Minute
)

type mongoSpanEntry struct {
	span      trace.Span
	createdAt time.Time
}

// NewCommandMonitor returns an event.CommandMonitor that creates a span per
// Mongo command. Spans are tracked by RequestID in a sync.Map keyed by
// (connectionID, requestID); a janitor goroutine purges entries older than
// mongoSpanTTL to avoid leaks if Succeeded/Failed never fires.
func NewCommandMonitor(stop <-chan struct{}) *event.CommandMonitor {
	tracer := otel.Tracer(mongoTracerName)
	var spans sync.Map // map[int64]*mongoSpanEntry

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				now := time.Now()
				spans.Range(func(k, v any) bool {
					if entry, ok := v.(*mongoSpanEntry); ok && now.Sub(entry.createdAt) > mongoSpanTTL {
						entry.span.SetStatus(codes.Error, "mongo command monitor leak (no Succeeded/Failed)")
						entry.span.End()
						spans.Delete(k)
					}
					return true
				})
			}
		}
	}()

	finish := func(reqID int64, err error) {
		v, ok := spans.LoadAndDelete(reqID)
		if !ok {
			return
		}
		entry := v.(*mongoSpanEntry)
		if err != nil {
			entry.span.RecordError(err)
			entry.span.SetStatus(codes.Error, err.Error())
		}
		entry.span.End()
	}

	return &event.CommandMonitor{
		Started: func(ctx context.Context, e *event.CommandStartedEvent) {
			_, span := tracer.Start(ctx, "mongo."+e.CommandName,
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(
					attribute.String("db.system", "mongodb"),
					attribute.String("db.name", e.DatabaseName),
					attribute.String("db.operation", e.CommandName),
				),
			)
			spans.Store(e.RequestID, &mongoSpanEntry{span: span, createdAt: time.Now()})
		},
		Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
			finish(e.RequestID, nil)
		},
		Failed: func(_ context.Context, e *event.CommandFailedEvent) {
			// e.Failure may be nil on older drivers; fall back to generic error
			var err error
			if e.Failure != nil {
				err = e.Failure
			}
			finish(e.RequestID, err)
		},
	}
}
