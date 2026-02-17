package httpmiddleware

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimitConfig configures the sliding window rate limiter.
type RateLimitConfig struct {
	// Max is the maximum number of requests allowed per window.
	Max int
	// Window is the duration of each sliding window.
	Window time.Duration
	// KeyFunc extracts the rate limit key from a request.
	// If nil, the client IP address is used.
	KeyFunc func(*http.Request) string
}

// entry tracks request counts across two adjacent windows for the sliding
// window algorithm.
type entry struct {
	prevCount float64
	prevStart time.Time
	currCount float64
	currStart time.Time
}

// rateLimiter holds the shared state for rate limiting.
type rateLimiter struct {
	cfg     RateLimitConfig
	mu      sync.Mutex
	entries map[string]*entry
}

func newRateLimiter(cfg RateLimitConfig) *rateLimiter {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = defaultKeyFunc
	}
	return &rateLimiter{
		cfg:     cfg,
		entries: make(map[string]*entry),
	}
}

// allow checks whether the request identified by key is within the rate limit.
// It returns the remaining request count, the window reset time, and whether
// the request is allowed. The caller must NOT hold rl.mu.
func (rl *rateLimiter) allow(key string, now time.Time) (remaining int, resetAt time.Time, allowed bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	e, ok := rl.entries[key]
	if !ok {
		e = &entry{currStart: now}
		rl.entries[key] = e
	}

	// Rotate window if the current window has elapsed.
	if now.Sub(e.currStart) >= rl.cfg.Window {
		e.prevCount = e.currCount
		e.prevStart = e.currStart
		e.currCount = 0
		e.currStart = now.Truncate(rl.cfg.Window)
		// If even the previous window is stale, zero it out.
		if now.Sub(e.prevStart) >= 2*rl.cfg.Window {
			e.prevCount = 0
		}
	}

	// Sliding window: weight previous window by how much of it overlaps
	// with the current sliding window.
	elapsed := now.Sub(e.currStart)
	overlapRatio := 1.0 - elapsed.Seconds()/rl.cfg.Window.Seconds()
	if overlapRatio < 0 {
		overlapRatio = 0
	}
	effectiveCount := e.prevCount*overlapRatio + e.currCount
	resetAt = e.currStart.Add(rl.cfg.Window)

	if effectiveCount >= float64(rl.cfg.Max) {
		return 0, resetAt, false
	}

	e.currCount++
	effectiveCount++

	remaining = int(float64(rl.cfg.Max) - effectiveCount)
	if remaining < 0 {
		remaining = 0
	}
	return remaining, resetAt, true
}

// cleanup removes entries whose windows have fully expired.
func (rl *rateLimiter) cleanup(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, e := range rl.entries {
		if now.Sub(e.currStart) >= 2*rl.cfg.Window {
			delete(rl.entries, key)
		}
	}
}

// startCleanup launches a background goroutine that periodically removes
// expired entries. It stops when ctx is cancelled.
func (rl *rateLimiter) startCleanup(ctx context.Context) {
	interval := 2 * rl.cfg.Window
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				rl.cleanup(now)
			}
		}
	}()
}

// RateLimit returns a middleware that enforces a per-key sliding window rate
// limit. When the limit is exceeded, it responds with 429 Too Many Requests
// and a JSON body. Every response includes X-RateLimit-Limit,
// X-RateLimit-Remaining, and X-RateLimit-Reset headers.
//
// This variant does not start a background cleanup goroutine. Use
// RateLimitWithCleanup if you need automatic eviction of stale entries.
func RateLimit(cfg RateLimitConfig) Middleware {
	rl := newRateLimiter(cfg)
	return rateLimitMiddleware(rl)
}

// RateLimitWithCleanup is like RateLimit but additionally starts a background
// goroutine that evicts expired entries every 2x the window duration. The
// goroutine stops when ctx is cancelled.
func RateLimitWithCleanup(ctx context.Context, cfg RateLimitConfig) Middleware {
	rl := newRateLimiter(cfg)
	rl.startCleanup(ctx)
	return rateLimitMiddleware(rl)
}

func rateLimitMiddleware(rl *rateLimiter) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rl.cfg.KeyFunc(r)
			now := time.Now()

			remaining, resetAt, allowed := rl.allow(key, now)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.cfg.Max))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if !allowed {
				retryAfter := time.Until(resetAt)
				if retryAfter < 0 {
					retryAfter = 0
				}
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    429,
					"message": "rate limit exceeded",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// defaultKeyFunc extracts the client IP from the request, checking
// X-Forwarded-For first, then X-Real-IP, then falling back to RemoteAddr.
func defaultKeyFunc(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For may contain a comma-separated list; use the first.
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
