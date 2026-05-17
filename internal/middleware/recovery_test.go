package middleware

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecovery_CatchesPanic_Returns500(t *testing.T) {
	r := gin.New()
	r.Use(Recovery(discardLogger()))
	r.GET("/boom", func(c *gin.Context) { panic("oops") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	require.NotPanics(t, func() { r.ServeHTTP(w, req) })
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRecovery_LogsStackOnPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	r := gin.New()
	r.Use(Recovery(logger))
	r.GET("/p", func(c *gin.Context) { panic(errors.New("boom-err")) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/p", nil))
	out := buf.String()
	assert.Contains(t, out, "panic recovered")
	assert.Contains(t, out, "/p")
}

func TestRecovery_NormalRequest_NoIntervention(t *testing.T) {
	r := gin.New()
	r.Use(Recovery(discardLogger()))
	r.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}
