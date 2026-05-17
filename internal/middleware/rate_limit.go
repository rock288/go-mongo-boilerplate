package middleware

import (
	"container/list"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/rock288/go-mongo-boilerplate/internal/platform/config"
)

// rateLimitEntry is the LRU value.
type rateLimitEntry struct {
	key      string
	limiter  *rate.Limiter
	lastSeen time.Time
}

// rateLimiterStore is a sync-safe LRU of per-key rate.Limiters. Cap prevents
// OOM when callers spoof their IP. TTL evicts cold keys.
type rateLimiterStore struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	maxSize int
	ttl     time.Duration
	entries map[string]*list.Element
	order   *list.List
}

func newRateLimiterStore(rps float64, burst, maxSize int, ttl time.Duration) *rateLimiterStore {
	return &rateLimiterStore{
		limit:   rate.Limit(rps),
		burst:   burst,
		maxSize: maxSize,
		ttl:     ttl,
		entries: map[string]*list.Element{},
		order:   list.New(),
	}
}

func (s *rateLimiterStore) get(key string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if el, ok := s.entries[key]; ok {
		e := el.Value.(*rateLimitEntry)
		e.lastSeen = now
		s.order.MoveToFront(el)
		return e.limiter
	}
	for s.order.Len() > 0 {
		back := s.order.Back()
		entry := back.Value.(*rateLimitEntry)
		if s.ttl > 0 && now.Sub(entry.lastSeen) > s.ttl {
			s.order.Remove(back)
			delete(s.entries, entry.key)
			continue
		}
		if s.order.Len() >= s.maxSize {
			s.order.Remove(back)
			delete(s.entries, entry.key)
			continue
		}
		break
	}
	l := rate.NewLimiter(s.limit, s.burst)
	el := s.order.PushFront(&rateLimitEntry{key: key, limiter: l, lastSeen: now})
	s.entries[key] = el
	return l
}

// RateLimit returns a gin handler that enforces per-IP and global token-bucket
// limits. Trusted proxies must be set on gin.Engine for c.ClientIP() to be
// reliable behind a load balancer.
func RateLimit(cfg config.RateLimitConfig) gin.HandlerFunc {
	if cfg.PerIP.RPS <= 0 && cfg.Global.RPS <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	maxSize := cfg.MapMaxEntries
	if maxSize <= 0 {
		maxSize = 50000
	}
	ttl := cfg.MapTTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	var perIP *rateLimiterStore
	if cfg.PerIP.RPS > 0 {
		perIP = newRateLimiterStore(cfg.PerIP.RPS, cfg.PerIP.Burst, maxSize, ttl)
	}
	var global *rate.Limiter
	if cfg.Global.RPS > 0 {
		global = rate.NewLimiter(rate.Limit(cfg.Global.RPS), cfg.Global.Burst)
	}

	return func(c *gin.Context) {
		if global != nil && !global.Allow() {
			tooMany(c, 1)
			return
		}
		if perIP != nil {
			ip := c.ClientIP()
			if ip == "" {
				ip = c.Request.RemoteAddr
			}
			if !perIP.get(ip).Allow() {
				tooMany(c, 1)
				return
			}
		}
		c.Next()
	}
}

func tooMany(c *gin.Context, retryAfterSec int) {
	c.Writer.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
}
