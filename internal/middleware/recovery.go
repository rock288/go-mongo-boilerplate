package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Recovery captures handler panics, logs the stack, records the error
// on the active OTel span, and returns 500. Must run inside the otelgin
// middleware so trace.SpanFromContext returns the request span.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("panic: %v", r)
				stack := debug.Stack()
				logger.ErrorContext(c.Request.Context(), "panic recovered",
					slog.String("error", err.Error()),
					slog.String("stack", string(stack)),
					slog.String("path", c.Request.URL.Path),
				)
				if span := trace.SpanFromContext(c.Request.Context()); span.SpanContext().IsValid() {
					span.RecordError(err, trace.WithStackTrace(true))
					span.SetStatus(codes.Error, err.Error())
				}
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
