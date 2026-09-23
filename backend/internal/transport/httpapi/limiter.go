package httpapi

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Bounded single-process limiter. Multi-replica deployment requires shared limiting.
type bucket struct {
	count int
	until time.Time
}
type limiter struct {
	mu      sync.Mutex
	entries map[string]bucket
}

func newLimiter() *limiter { return &limiter{entries: make(map[string]bucket)} }
func (l *limiter) take(key string, max int, window time.Duration, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) >= 10000 {
		for key, b := range l.entries {
			if !b.until.After(now) {
				delete(l.entries, key)
			}
		}
	}
	b, exists := l.entries[key]
	if !exists && len(l.entries) >= 10000 {
		return false, 60
	}
	if !b.until.After(now) {
		b = bucket{until: now.Add(window)}
	}
	if b.count >= max {
		return false, int(b.until.Sub(now).Seconds()) + 1
	}
	b.count++
	l.entries[key] = b
	return true, 0
}
func (api *API) allow(c *gin.Context, key string, max int, window time.Duration) bool {
	ok, retry := api.limits.take(key, max, window, time.Now())
	if !ok {
		c.Header("Retry-After", strconv.Itoa(retry))
		problem(c, 429, "rate_limited")
	}
	return ok
}
