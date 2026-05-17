package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyLimit_RejectsOversize(t *testing.T) {
	r := gin.New()
	r.Use(BodyLimit(10))
	r.POST("/x", func(c *gin.Context) {
		buf := make([]byte, 1024)
		if _, err := c.Request.Body.Read(buf); err != nil {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewReader([]byte(strings.Repeat("a", 100))))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestBodyLimit_AcceptsUnderLimit(t *testing.T) {
	r := gin.New()
	r.Use(BodyLimit(100))
	r.POST("/x", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatus(http.StatusRequestEntityTooLarge)
			return
		}
		c.String(http.StatusOK, string(body))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("hello"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBodyLimit_ZeroDisablesLimit(t *testing.T) {
	r := gin.New()
	r.Use(BodyLimit(0))
	r.POST("/x", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 100000))))
	assert.Equal(t, http.StatusOK, w.Code)
}
