package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit caps inbound request body size. Applied before JSON bind so
// oversized payloads short-circuit to 413.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
