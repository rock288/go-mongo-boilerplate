package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HeaderRequestID is the canonical request-id header name. Some upstream LBs
// use X-Amzn-Trace-Id / X-Cloud-Trace-Context — adapt here if you front this
// service with one and want to honour their value.
const HeaderRequestID = "X-Request-ID"

type requestIDCtxKey struct{}

// RequestID extracts X-Request-ID from the incoming request or generates a
// new UUID if absent. The value is stored on the gin context, propagated to
// the request context (so downstream Mongo/Kafka/HTTP calls inherit it), and
// echoed back in the response header.
//
// Wire BEFORE RequestLogger so the logger can read the value via FromContext.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(string(HeaderRequestID), id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDCtxKey{}, id))
		c.Writer.Header().Set(HeaderRequestID, id)
		c.Next()
	}
}

// FromContext returns the request id stored on ctx, or "" if absent.
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDCtxKey{}).(string); ok {
		return v
	}
	return ""
}
