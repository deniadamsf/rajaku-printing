package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/rajaku-printing/backend/internal/httpx"
)

// RateLimitPublic is a per-client-IP token-bucket limiter for public endpoints
// (e.g., /lacak/:resi — per spec section 23, must not be a brute-force vector
// for guessing resi numbers).
//
// NOTE: This is a single-process, in-memory limiter — sufficient for MVP with
// one API instance. When scaling to multiple instances, migrate to Redis-backed
// limiter (spec section 13: "trigger for Redis").
func RateLimitPublic(rps float64, burst int) gin.HandlerFunc {
	l := newIPLimiter(rate.Limit(rps), burst)

	go l.gcLoop(10*time.Minute, 30*time.Minute)

	return func(c *gin.Context) {
		if !l.allow(c.ClientIP()) {
			httpx.Error(c, http.StatusTooManyRequests, httpx.CodeRateLimited, "too many requests")
			return
		}
		c.Next()
	}
}

type ipLimiter struct {
	mu       sync.Mutex
	limiters map[string]*limiterEntry
	rps      rate.Limit
	burst    int
}

type limiterEntry struct {
	l        *rate.Limiter
	lastSeen time.Time
}

func newIPLimiter(rps rate.Limit, burst int) *ipLimiter {
	return &ipLimiter{
		limiters: make(map[string]*limiterEntry),
		rps:      rps,
		burst:    burst,
	}
}

func (i *ipLimiter) allow(ip string) bool {
	i.mu.Lock()
	entry, ok := i.limiters[ip]
	if !ok {
		entry = &limiterEntry{l: rate.NewLimiter(i.rps, i.burst)}
		i.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	i.mu.Unlock()
	return entry.l.Allow()
}

func (i *ipLimiter) gcLoop(interval, ttl time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-ttl)
		i.mu.Lock()
		for ip, e := range i.limiters {
			if e.lastSeen.Before(cutoff) {
				delete(i.limiters, ip)
			}
		}
		i.mu.Unlock()
	}
}
