package main

import (
	"context"

	"github.com/go-faster/sdk/app"
	"go.uber.org/zap"

	appkg "github.com/xenking/oolio-kart-challenge/internal/app"
)

func main() {
	app.Run(func(ctx context.Context, lg *zap.Logger, m *app.Telemetry) error {
		cfg, err := appkg.LoadConfig()
		if err != nil {
			return err
		}
		return appkg.Run(ctx, lg, m, cfg)
	})
}
