package sandbox

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string][]time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	if max < 1 {
		max = 1
	}
	if window <= 0 {
		window = time.Hour
	}
	return &Limiter{
		max:    max,
		window: window,
		now:    time.Now,
		hits:   map[string][]time.Time{},
	}
}

func (l *Limiter) Allow(key string) (int, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.prune(key)
	if len(kept) >= l.max {
		l.hits[key] = kept
		return 0, false
	}
	kept = append(kept, l.now())
	l.hits[key] = kept
	return l.max - len(kept), true
}

func (l *Limiter) Remaining(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.prune(key)
	l.hits[key] = kept
	left := l.max - len(kept)
	if left < 0 {
		return 0
	}
	return left
}

func (l *Limiter) prune(key string) []time.Time {
	cutoff := l.now().Add(-l.window)
	var kept []time.Time
	for _, seen := range l.hits[key] {
		if seen.After(cutoff) {
			kept = append(kept, seen)
		}
	}
	return kept
}

func Gate(enabled bool, limiter *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}
		if _, ok := limiter.Allow(ClientKey(c)); !ok {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":            "sandbox quota exceeded",
				"remainingIpQuota": 0,
			})
			return
		}
		c.Next()
	}
}

func ClientKey(c *gin.Context) string {
	if forwarded := c.GetHeader("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return c.ClientIP()
}
