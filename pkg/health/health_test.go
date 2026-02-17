package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-faster/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func passingCheck() CheckFunc {
	return func(_ context.Context) error {
		return nil
	}
}

func failingCheck(msg string) CheckFunc {
	return func(_ context.Context) error {
		return errors.New(msg)
	}
}

func TestLiveEndpoint_AllPassing(t *testing.T) {
	h := New()
	h.AddLivenessCheck("check1", time.Second, passingCheck())
	h.AddLivenessCheck("check2", time.Second, passingCheck())

	// Checks start healthy by default.
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()

	h.LiveEndpoint(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body statusResponse
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body.Status)
}

func TestLiveEndpoint_FailingCheck(t *testing.T) {
	h := New()
	h.AddLivenessCheck("db", time.Second, failingCheck("connection refused"))

	// The check starts as healthy. We need to drive it past the failure
	// threshold (3 consecutive failures) for it to flip to unhealthy.
	ctx := context.Background()
	h.livenessChecks[0].run(ctx)
	h.livenessChecks[0].run(ctx)
	h.livenessChecks[0].run(ctx)

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()

	h.LiveEndpoint(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body statusResponse
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", body.Status)
	assert.Contains(t, body.Checks, "db")
	assert.Equal(t, "connection refused", body.Checks["db"])
}

func TestLiveEndpoint_FailureBelowThreshold(t *testing.T) {
	h := New()
	h.AddLivenessCheck("flaky", time.Second, failingCheck("temporary"))

	// Only 2 failures, threshold is 3. Should still be healthy.
	ctx := context.Background()
	h.livenessChecks[0].run(ctx)
	h.livenessChecks[0].run(ctx)

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()

	h.LiveEndpoint(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestReadyEndpoint_ReadyAndPassing(t *testing.T) {
	h := New()
	h.AddReadinessCheck("cache", time.Second, passingCheck())
	h.SetReady(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	h.ReadyEndpoint(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body statusResponse
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body.Status)
}

func TestReadyEndpoint_NotReady(t *testing.T) {
	h := New()
	h.AddReadinessCheck("cache", time.Second, passingCheck())
	// Do NOT call SetReady(true); default is not ready.

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	h.ReadyEndpoint(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body statusResponse
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", body.Status)
	assert.Contains(t, body.Checks, "_readiness")
}

func TestReadyEndpoint_SetReadyFalse(t *testing.T) {
	h := New()
	h.AddReadinessCheck("cache", time.Second, passingCheck())
	h.SetReady(true)

	// Verify it is ready.
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	h.ReadyEndpoint(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Set not ready.
	h.SetReady(false)

	req2 := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w2 := httptest.NewRecorder()
	h.ReadyEndpoint(w2, req2)
	assert.Equal(t, http.StatusServiceUnavailable, w2.Code)
}

func TestReadyEndpoint_MultipleChecksOneFailing(t *testing.T) {
	h := New()
	h.AddReadinessCheck("db", time.Second, passingCheck())
	h.AddReadinessCheck("cache", time.Second, failingCheck("cache miss"))
	h.SetReady(true)

	// Drive the cache check past the failure threshold.
	ctx := context.Background()
	h.readinessChecks[1].run(ctx)
	h.readinessChecks[1].run(ctx)
	h.readinessChecks[1].run(ctx)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	h.ReadyEndpoint(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body statusResponse
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "unhealthy", body.Status)
	assert.Contains(t, body.Checks, "cache")
	assert.NotContains(t, body.Checks, "db")
}

func TestIsReady(t *testing.T) {
	h := New()
	h.AddReadinessCheck("db", time.Second, passingCheck())

	assert.False(t, h.IsReady(), "should not be ready before SetReady")

	h.SetReady(true)
	assert.True(t, h.IsReady(), "should be ready after SetReady(true)")

	h.SetReady(false)
	assert.False(t, h.IsReady(), "should not be ready after SetReady(false)")
}

func TestStopCancelsChecks(t *testing.T) {
	h := New()
	h.AddLivenessCheck("goroutine", time.Second, passingCheck())

	ctx := context.Background()
	h.Start(ctx, 100*time.Millisecond)

	// Give the check goroutine a moment to run.
	time.Sleep(50 * time.Millisecond)

	// Stop should not panic and should be idempotent.
	h.Stop()
	h.Stop()
}

func TestLiveEndpoint_NoChecks(t *testing.T) {
	h := New()

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()

	h.LiveEndpoint(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body statusResponse
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "ok", body.Status)
}

func TestReadyEndpoint_NoChecksButReady(t *testing.T) {
	h := New()
	h.SetReady(true)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	h.ReadyEndpoint(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCheckRecovery(t *testing.T) {
	// A check that starts failing then recovers should become healthy again
	// after successThreshold consecutive successes.
	failing := true
	h := New()
	h.AddLivenessCheck("flaky", time.Second, func(_ context.Context) error {
		if failing {
			return errors.New("down")
		}
		return nil
	})
	c := h.livenessChecks[0]
	ctx := context.Background()

	// Drive past failure threshold (3).
	c.run(ctx)
	c.run(ctx)
	c.run(ctx)
	assert.False(t, c.isHealthy())

	// Switch to passing. One success should recover (successThreshold = 1).
	failing = false
	c.run(ctx)
	assert.True(t, c.isHealthy(), "check should recover after successThreshold consecutive passes")
}

func TestCheckLastErrorStored(t *testing.T) {
	h := New()
	h.AddLivenessCheck("db", time.Second, failingCheck("timeout"))
	c := h.livenessChecks[0]

	// Before any run, no last error.
	assert.Nil(t, c.getLastError())

	// After run, last error should be stored.
	c.run(context.Background())
	assert.EqualError(t, c.getLastError(), "timeout")
}

func TestConcurrentAccess(t *testing.T) {
	// Verify no races when run() and isHealthy()/getLastError() are called concurrently.
	h := New()
	h.AddLivenessCheck("concurrent", time.Second, failingCheck("err"))
	h.AddReadinessCheck("concurrent", time.Second, passingCheck())
	h.SetReady(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.Start(ctx, 10*time.Millisecond)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				h.IsReady()

				req := httptest.NewRequest(http.MethodGet, "/livez", nil)
				w := httptest.NewRecorder()
				h.LiveEndpoint(w, req)

				req = httptest.NewRequest(http.MethodGet, "/readyz", nil)
				w = httptest.NewRecorder()
				h.ReadyEndpoint(w, req)
			}
		}()
	}
	wg.Wait()
	h.Stop()
}

func TestGoroutineCountCheck(t *testing.T) {
	// With a very high threshold, the check should pass.
	check := GoroutineCountCheck(100000)
	assert.NoError(t, check(context.Background()))

	// With a threshold of 0, it should always fail.
	check = GoroutineCountCheck(0)
	err := check(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds threshold")
}

func TestGCMaxPauseCheck(t *testing.T) {
	// With a very generous threshold, the check should pass.
	check := GCMaxPauseCheck(time.Hour)
	assert.NoError(t, check(context.Background()))
}
