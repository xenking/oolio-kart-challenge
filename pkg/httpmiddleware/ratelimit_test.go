package httpmiddleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRateLimit_UnderLimit(t *testing.T) {
	cfg := RateLimitConfig{
		Max:    5,
		Window: time.Minute,
	}
	handler := RateLimit(cfg)(okHandler())

	for i := range 5 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, "request %d should pass", i+1)
		assert.Equal(t, "5", w.Header().Get("X-RateLimit-Limit"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
	}
}

func TestRateLimit_OverLimit(t *testing.T) {
	cfg := RateLimitConfig{
		Max:    2,
		Window: time.Minute,
	}
	handler := RateLimit(cfg)(okHandler())

	// Exhaust the limit.
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	// Next request should be rate limited.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Equal(t, "0", w.Header().Get("X-RateLimit-Remaining"))

	var body map[string]any
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, float64(429), body["code"])
	assert.Equal(t, "rate limit exceeded", body["message"])
}

func TestRateLimit_DifferentIPs(t *testing.T) {
	cfg := RateLimitConfig{
		Max:    1,
		Window: time.Minute,
	}
	handler := RateLimit(cfg)(okHandler())

	// First IP should succeed.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second IP should also succeed (independent limit).
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.2:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// First IP again should be rate limited.
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "10.0.0.1:5678"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusTooManyRequests, w3.Code)
}

func TestRateLimit_Headers(t *testing.T) {
	cfg := RateLimitConfig{
		Max:    10,
		Window: time.Minute,
	}
	handler := RateLimit(cfg)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:4444"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, "10", w.Header().Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
}

func TestRateLimit_CustomKeyFunc(t *testing.T) {
	cfg := RateLimitConfig{
		Max:    1,
		Window: time.Minute,
		KeyFunc: func(r *http.Request) string {
			return r.Header.Get("X-API-Key")
		},
	}
	handler := RateLimit(cfg)(okHandler())

	// First key succeeds.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.Header.Set("X-API-Key", "key-a")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Same key gets limited.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-API-Key", "key-a")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)

	// Different key succeeds.
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.Header.Set("X-API-Key", "key-b")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
}

func TestRateLimit_XForwardedFor(t *testing.T) {
	cfg := RateLimitConfig{
		Max:    1,
		Window: time.Minute,
	}
	handler := RateLimit(cfg)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:4444"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Same X-Forwarded-For first IP should be limited.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.2:5555" // different RemoteAddr
	req2.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
}
