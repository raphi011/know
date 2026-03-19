package api

import (
	"log/slog"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/metrics"
)

// IPRateLimiter provides per-IP rate limiting using a token bucket algorithm.
type IPRateLimiter struct {
	limiters sync.Map // map[string]*limiterEntry
	rps      rate.Limit
	burst    int
	tier     string // "auth" or "global", for metrics
	metrics  *metrics.Metrics
	done     chan struct{}
}

type limiterEntry struct {
	limiter      *rate.Limiter
	lastSeenUnix atomic.Int64 // unix timestamp, accessed concurrently
}

// NewIPRateLimiter creates a new per-IP rate limiter.
// rps is the sustained requests per second; burst is the max burst size.
// tier is used for metrics labels ("auth" or "global").
// Starts a background goroutine to clean up stale entries every 10 minutes.
func NewIPRateLimiter(rps float64, burst int, tier string, m *metrics.Metrics) *IPRateLimiter {
	rl := &IPRateLimiter{
		rps:     rate.Limit(rps),
		burst:   burst,
		tier:    tier,
		metrics: m,
		done:    make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop terminates the background cleanup goroutine.
func (rl *IPRateLimiter) Stop() {
	close(rl.done)
}

// Middleware returns HTTP middleware that rate-limits requests by client IP.
// Returns 429 Too Many Requests with a Retry-After header when the limit is exceeded.
func (rl *IPRateLimiter) Middleware(trustXFF bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := httputil.ClientIP(r, trustXFF)
			limiter := rl.getLimiter(ip)

			if !limiter.Allow() {
				rl.metrics.RecordRateLimitRejection(rl.tier)
				slog.Debug("rate limit exceeded", "tier", rl.tier, "ip", ip, "path", r.URL.Path)
				retryAfter := int(math.Ceil(1.0 / float64(rl.rps)))
				httputil.WriteProblemWithRetry(w, http.StatusTooManyRequests, "rate limit exceeded", retryAfter)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (rl *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	now := time.Now().Unix()
	entry := &limiterEntry{limiter: rate.NewLimiter(rl.rps, rl.burst)}
	entry.lastSeenUnix.Store(now)
	if v, loaded := rl.limiters.LoadOrStore(ip, entry); loaded {
		existing := v.(*limiterEntry)
		existing.lastSeenUnix.Store(now)
		return existing.limiter
	}
	return entry.limiter
}

// cleanup removes limiter entries not seen in 10 minutes.
func (rl *IPRateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-10 * time.Minute).Unix()
			rl.limiters.Range(func(key, value any) bool {
				entry := value.(*limiterEntry)
				if entry.lastSeenUnix.Load() < cutoff {
					rl.limiters.Delete(key)
				}
				return true
			})
		case <-rl.done:
			return
		}
	}
}
