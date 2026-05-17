package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rock288/go-mongo-boilerplate/internal/middleware"
)

func init() { gin.SetMode(gin.TestMode) }

func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	t.Parallel()

	r := gin.New()
	r.Use(middleware.RequestID())
	var seen string
	r.GET("/", func(c *gin.Context) {
		seen = middleware.FromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, seen)
	assert.Equal(t, seen, w.Header().Get(middleware.HeaderRequestID))
}

func TestRequestID_HonoursIncomingHeader(t *testing.T) {
	t.Parallel()

	const given = "req-abc-123"
	r := gin.New()
	r.Use(middleware.RequestID())
	var seen string
	r.GET("/", func(c *gin.Context) {
		seen = middleware.FromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(middleware.HeaderRequestID, given)
	r.ServeHTTP(w, req)

	assert.Equal(t, given, seen)
	assert.Equal(t, given, w.Header().Get(middleware.HeaderRequestID))
}

func TestFromContext_AbsentReturnsEmpty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, middleware.FromContext(t.Context()))
}
