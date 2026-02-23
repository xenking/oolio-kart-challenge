package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/go-faster/sdk/app"
	"go.uber.org/zap"

	appkg "github.com/xenking/oolio-kart-challenge/internal/app"
)

func main() {
	// app.Run only listens for SIGINT. Docker sends SIGTERM on container stop.
	// Cancel the base context on SIGTERM so the graceful shutdown path runs
	// and the process exits cleanly (flushing coverage data when built with -cover).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	app.Run(func(ctx context.Context, lg *zap.Logger, m *app.Telemetry) error {
		cfg, err := appkg.LoadConfig()
		if err != nil {
			return err
		}
		return appkg.Run(ctx, lg, m, cfg)
	}, app.WithContext(ctx))
}
