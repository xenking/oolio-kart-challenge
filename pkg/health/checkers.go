package health

import (
	"context"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/go-faster/errors"
)

// GoroutineCountCheck returns a CheckFunc that reports unhealthy when the
// number of goroutines exceeds the given threshold. This is useful as a
// liveness check to detect goroutine leaks.
func GoroutineCountCheck(threshold int) CheckFunc {
	return func(_ context.Context) error {
		count := runtime.NumGoroutine()
		if count > threshold {
			return errors.Errorf("goroutine count %d exceeds threshold %d", count, threshold)
		}
		return nil
	}
}

// GCMaxPauseCheck returns a CheckFunc that reports unhealthy when the maximum
// GC pause (stop-the-world) duration exceeds the given threshold. This is
// useful as a liveness check to detect memory pressure or excessively large
// heaps causing long GC pauses.
func GCMaxPauseCheck(threshold time.Duration) CheckFunc {
	return func(_ context.Context) error {
		var stats debug.GCStats
		debug.ReadGCStats(&stats)

		for _, pause := range stats.Pause {
			if pause > threshold {
				return errors.Errorf("GC pause %s exceeds threshold %s", pause, threshold)
			}
		}
		return nil
	}
}
