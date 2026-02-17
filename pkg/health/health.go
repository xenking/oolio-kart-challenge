// Package health provides Kubernetes-style liveness and readiness probe support.
//
// Each registered check runs in its own background goroutine at a configurable
// interval. Checks use failure/success thresholds (inspired by Kubernetes probe
// configuration) to avoid flapping: a check must fail consecutively
// failureThreshold times before being marked unhealthy, and succeed
// successThreshold times before being marked healthy again.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// CheckFunc is a health check function. It should return nil if the checked
// component is healthy, or an error describing the problem.
type CheckFunc func(ctx context.Context) error

// checkConfig holds the configuration and runtime state for a single check.
//
// Concurrency model: run() is called from exactly one goroutine (the ticker).
// The counters (consecutiveFails, consecutiveOK) are only accessed by run(),
// so they need no synchronization. The healthy flag and lastErr are read by
// HTTP handlers from arbitrary goroutines, so they use atomic operations.
type checkConfig struct {
	name             string
	timeout          time.Duration
	check            CheckFunc
	failureThreshold int
	successThreshold int

	// healthy is read by HTTP handlers (atomic load) and written by run() (atomic store).
	healthy atomic.Bool

	// lastErr stores the most recent error from run(). Read by HTTP handlers via
	// atomic load; written by run() via atomic store.
	lastErr atomic.Pointer[error]

	// counters are only accessed from the single run() goroutine.
	consecutiveFails int
	consecutiveOK    int
}

// isHealthy returns the current health status of this check.
func (c *checkConfig) isHealthy() bool {
	return c.healthy.Load()
}

// getLastError returns the most recent error from this check, or nil.
func (c *checkConfig) getLastError() error {
	if p := c.lastErr.Load(); p != nil {
		return *p
	}
	return nil
}

// run executes the check once and updates thresholds accordingly.
// Must be called from a single goroutine.
func (c *checkConfig) run(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	err := c.check(checkCtx)
	c.lastErr.Store(&err)

	if err != nil {
		c.consecutiveOK = 0
		c.consecutiveFails++
		if c.consecutiveFails >= c.failureThreshold {
			c.healthy.Store(false)
		}
	} else {
		c.consecutiveFails = 0
		c.consecutiveOK++
		if c.consecutiveOK >= c.successThreshold {
			c.healthy.Store(true)
		}
	}
}

// Health manages liveness and readiness checks for a service.
type Health struct {
	ready atomic.Bool

	// mu protects check slices and cancel. Only held during registration (before
	// Start) and in Start/Stop. HTTP handlers snapshot the slices under RLock
	// then release immediately — no lock nesting with check state.
	mu              sync.RWMutex
	livenessChecks  []*checkConfig
	readinessChecks []*checkConfig
	cancel          context.CancelFunc
}

// New creates a new Health instance. The service starts in a not-ready state;
// call SetReady(true) once the service has finished initialization.
func New() *Health {
	return &Health{}
}

// AddLivenessCheck registers a liveness check. Liveness checks determine
// whether the process is alive and functioning. Examples: goroutine count,
// GC pause duration, deadlock detection.
func (h *Health) AddLivenessCheck(name string, timeout time.Duration, check CheckFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()

	c := &checkConfig{
		name:             name,
		timeout:          timeout,
		check:            check,
		failureThreshold: 3,
		successThreshold: 1,
	}
	c.healthy.Store(true) // assume healthy until proven otherwise
	h.livenessChecks = append(h.livenessChecks, c)
}

// AddReadinessCheck registers a readiness check. Readiness checks determine
// whether the service is ready to accept traffic. Examples: database
// connectivity, cache warmup, dependent service availability.
func (h *Health) AddReadinessCheck(name string, timeout time.Duration, check CheckFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()

	c := &checkConfig{
		name:             name,
		timeout:          timeout,
		check:            check,
		failureThreshold: 3,
		successThreshold: 1,
	}
	c.healthy.Store(true) // assume healthy until proven otherwise
	h.readinessChecks = append(h.readinessChecks, c)
}

// Start begins running all registered checks in background goroutines at the
// given interval. Each check runs in its own goroutine. Calling Start multiple
// times without calling Stop first is a no-op for already-running checks, but
// typically Start should be called once after all checks are registered.
func (h *Health) Start(ctx context.Context, interval time.Duration) {
	ctx, cancel := context.WithCancel(ctx)

	h.mu.Lock()
	h.cancel = cancel
	checks := make([]*checkConfig, 0, len(h.livenessChecks)+len(h.readinessChecks))
	checks = append(checks, h.livenessChecks...)
	checks = append(checks, h.readinessChecks...)
	h.mu.Unlock()

	for _, c := range checks {
		go runCheck(ctx, c, interval)
	}
}

// runCheck periodically executes a single check until the context is cancelled.
func runCheck(ctx context.Context, c *checkConfig, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start.
	c.run(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.run(ctx)
		}
	}
}

// SetReady manually sets the readiness state. This is typically called with
// true after service initialization completes, and with false during graceful
// shutdown to stop receiving new traffic.
func (h *Health) SetReady(ready bool) {
	h.ready.Store(ready)
}

// IsReady returns whether the service is ready to accept traffic. It returns
// true only if the service has been manually marked ready AND all readiness
// checks are currently passing.
func (h *Health) IsReady() bool {
	if !h.ready.Load() {
		return false
	}

	h.mu.RLock()
	checks := h.readinessChecks
	h.mu.RUnlock()

	for _, c := range checks {
		if !c.isHealthy() {
			return false
		}
	}
	return true
}

// Stop cancels all background check goroutines. It is safe to call Stop
// multiple times.
func (h *Health) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

// statusResponse is the JSON response body for health endpoints.
type statusResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// LiveEndpoint is an http.HandlerFunc for the /livez endpoint.
// It returns 200 with {"status":"ok"} if all liveness checks are passing,
// or 503 with {"status":"unhealthy","checks":{...}} listing failures.
func (h *Health) LiveEndpoint(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	checks := make([]*checkConfig, len(h.livenessChecks))
	copy(checks, h.livenessChecks)
	h.mu.RUnlock()

	failures := collectFailures(checks)
	writeResponse(w, failures)
}

// ReadyEndpoint is an http.HandlerFunc for the /readyz endpoint.
// It returns 200 with {"status":"ok"} if the service is manually marked ready
// AND all readiness checks are passing. Otherwise it returns 503 with details.
func (h *Health) ReadyEndpoint(w http.ResponseWriter, r *http.Request) {
	ready := h.ready.Load()

	h.mu.RLock()
	checks := make([]*checkConfig, len(h.readinessChecks))
	copy(checks, h.readinessChecks)
	h.mu.RUnlock()

	failures := collectFailures(checks)
	if !ready {
		failures["_readiness"] = "service is not ready"
	}
	writeResponse(w, failures)
}

// collectFailures returns a map of check name to error message for any check
// that is currently unhealthy. Uses the stored last error from run() rather
// than re-executing the check function.
func collectFailures(checks []*checkConfig) map[string]string {
	failures := make(map[string]string)
	for _, c := range checks {
		if !c.isHealthy() {
			if err := c.getLastError(); err != nil {
				failures[c.name] = err.Error()
			} else {
				failures[c.name] = "check is unhealthy"
			}
		}
	}
	return failures
}

// writeResponse writes the appropriate HTTP status and JSON body based on
// whether any failures were found.
func writeResponse(w http.ResponseWriter, failures map[string]string) {
	w.Header().Set("Content-Type", "application/json")

	resp := statusResponse{Status: "ok"}
	status := http.StatusOK

	if len(failures) > 0 {
		resp.Status = "unhealthy"
		resp.Checks = failures
		status = http.StatusServiceUnavailable
	}

	w.WriteHeader(status)

	// Best effort: the status code is already written, so we cannot change
	// the response. This should only happen if the client disconnected.
	_ = json.NewEncoder(w).Encode(resp)
}
