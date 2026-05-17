package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeout_AttachesDeadlineToContext(t *testing.T) {
	r := gin.New()
	r.Use(Timeout(50 * time.Millisecond))
	var sawDeadline bool
	r.GET("/x", func(c *gin.Context) {
		_, ok := c.Request.Context().Deadline()
		sawDeadline = ok
		c.Status(http.StatusOK)
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.True(t, sawDeadline, "Timeout middleware must attach a deadline")
}

func TestTimeout_ZeroDuration_Passthrough(t *testing.T) {
	r := gin.New()
	r.Use(Timeout(0))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTimeout_PropagatesCancellation(t *testing.T) {
	r := gin.New()
	r.Use(Timeout(10 * time.Millisecond))
	var ctxErr error
	r.GET("/slow", func(c *gin.Context) {
		select {
		case <-time.After(50 * time.Millisecond):
			c.Status(http.StatusOK)
		case <-c.Request.Context().Done():
			ctxErr = c.Request.Context().Err()
			c.AbortWithStatus(http.StatusGatewayTimeout)
		}
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
	require.Error(t, ctxErr)
	assert.True(t, errors.Is(ctxErr, context.DeadlineExceeded))
}
