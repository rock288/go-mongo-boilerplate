package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

func TestRateLimit_ConcurrentMapSafe(t *testing.T) {
	cfg := config.RateLimitConfig{
		PerIP:         config.RateBucket{RPS: 1000, Burst: 1000},
		MapMaxEntries: 100,
	}
	handler := RateLimit(cfg)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			c.Request.RemoteAddr = "10.0.0.1:1234"
			handler(c)
		}()
	}
	wg.Wait()
}

func TestRateLimit_RejectsBurstOverflow(t *testing.T) {
	cfg := config.RateLimitConfig{
		PerIP:         config.RateBucket{RPS: 1, Burst: 1},
		MapMaxEntries: 10,
	}
	r := gin.New()
	r.Use(RateLimit(cfg))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	doReq := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "10.0.0.5:1234"
		r.ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusOK, doReq())
	assert.Equal(t, http.StatusTooManyRequests, doReq())
}

func TestRateLimit_DisabledWhenBothConfigsZero(t *testing.T) {
	cfg := config.RateLimitConfig{
		PerIP:  config.RateBucket{},
		Global: config.RateBucket{},
	}
	r := gin.New()
	r.Use(RateLimit(cfg))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		require.Equal(t, http.StatusOK, w.Code, "rate limit must be off when both buckets are zero")
	}
}

func TestRateLimit_GlobalRejectsAcrossIPs(t *testing.T) {
	cfg := config.RateLimitConfig{
		Global: config.RateBucket{RPS: 1, Burst: 1},
	}
	r := gin.New()
	r.Use(RateLimit(cfg))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	doReq := func(ip string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = ip + ":1234"
		r.ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusOK, doReq("1.1.1.1"))
	assert.Equal(t, http.StatusTooManyRequests, doReq("2.2.2.2"))
}
