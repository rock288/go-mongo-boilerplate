package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRequestLogger_LogsMethodPathStatusLatency(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	r := gin.New()
	r.Use(RequestLogger(logger))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusTeapot) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	r.ServeHTTP(w, req)

	out := buf.String()
	assert.Contains(t, out, `"method":"GET"`)
	assert.Contains(t, out, `"path":"/x"`)
	assert.Contains(t, out, `"status":418`)
	assert.Contains(t, out, `"latency"`)
}
