package main

import (
	"context"
	"net/http"
	"time"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigyaml"
	"github.com/go-faster/errors"
	"github.com/go-faster/sdk/app"
	"github.com/go-faster/sdk/zctx"
	"go.uber.org/zap"

	"github.com/xenking/oolio-kart-challenge/internal/api"
	"github.com/xenking/oolio-kart-challenge/internal/coupon"
	"github.com/xenking/oolio-kart-challenge/internal/oas"
	"github.com/xenking/oolio-kart-challenge/internal/postgres"
	"github.com/xenking/oolio-kart-challenge/pkg/health"
	"github.com/xenking/oolio-kart-challenge/pkg/httpmiddleware"
)

func main() {
	app.Run(func(ctx context.Context, lg *zap.Logger, m *app.Telemetry) error {
		var cfg Config
		loader := aconfig.LoaderFor(&cfg, aconfig.Config{
			EnvPrefix: "KART",
			Files:     []string{"config.yaml", "/etc/kart/config.yaml"},
			FileDecoders: map[string]aconfig.FileDecoder{
				".yaml": aconfigyaml.New(),
			},
		})
		if err := loader.Load(); err != nil {
			return errors.Wrap(err, "load config")
		}
		cfg.applyPlatformDefaults()

		lg.Info("Initializing", zap.String("addr", cfg.Addr))

		// PostgreSQL pool + migrations.
		pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
		if err != nil {
			return errors.Wrap(err, "create db pool")
		}
		defer pool.Close()

		if err := postgres.RunMigrations(ctx, pool); err != nil {
			return errors.Wrap(err, "run migrations")
		}

		// Health check service.
		healthSvc := health.New()
		healthSvc.AddReadinessCheck("postgres", 5*time.Second, func(ctx context.Context) error {
			return pool.Ping(ctx)
		})
		healthSvc.AddLivenessCheck("goroutines", time.Second, health.GoroutineCountCheck(10000))
		healthSvc.Start(ctx, 10*time.Second)
		healthSvc.SetReady(true)

		// Repositories.
		productRepo := postgres.NewProductRepository(pool)
		couponRepo := postgres.NewCouponRepository(pool)
		orderRepo := postgres.NewOrderRepository(pool)
		apikeyRepo := &apiKeyAdapter{repo: postgres.NewAPIKeyRepository(pool)}

		// API handler (implements both oas.Handler and oas.SecurityHandler).
		handler := api.NewHandler(
			api.HandlerConfig{ImageBaseURL: cfg.ImageBaseURL},
			productRepo, coupon.NewRepoValidator(couponRepo), orderRepo, apikeyRepo,
		)

		oasServer, err := oas.NewServer(handler, handler,
			oas.WithTracerProvider(m.TracerProvider()),
			oas.WithMeterProvider(m.MeterProvider()),
		)
		if err != nil {
			return errors.Wrap(err, "create oas server")
		}

		// Mux: health endpoints + ogen API routes on one server.
		routeFinder := httpmiddleware.MakeRouteFinder(oasServer)
		mux := http.NewServeMux()
		mux.HandleFunc("/livez", healthSvc.LiveEndpoint)
		mux.HandleFunc("/readyz", healthSvc.ReadyEndpoint)
		mux.Handle("/", oasServer)

		server := &http.Server{
			ReadHeaderTimeout: time.Second,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20,
			Addr:              cfg.Addr,
			Handler: httpmiddleware.Wrap(mux,
				httpmiddleware.CORS(httpmiddleware.CORSConfig{
					AllowOrigins:     cfg.CORS.Origins,
					AllowHeaders:     []string{"Content-Type", "Authorization", "api_key"},
					AllowCredentials: cfg.CORS.AllowCredentials,
					MaxAge:           86400,
				}),
				httpmiddleware.RateLimitWithCleanup(ctx, httpmiddleware.RateLimitConfig{
					Max:    cfg.RateLimit.Max,
					Window: cfg.RateLimit.Window,
				}),
				httpmiddleware.RequestID(),
				httpmiddleware.InjectLogger(zctx.From(ctx)),
				httpmiddleware.Instrument("kart-api", routeFinder, m),
				httpmiddleware.LogRequests(routeFinder),
				httpmiddleware.Labeler(routeFinder),
			),
		}

		// Graceful shutdown: wait for context cancellation, drain, then stop.
		shutdownDone := make(chan struct{})
		go func() {
			<-ctx.Done()
			healthSvc.SetReady(false)
			lg.Info("Readiness set to false, draining", zap.Duration("delay", cfg.Graceful.ReadinessDelay))
			time.Sleep(cfg.Graceful.ReadinessDelay)

			shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Graceful.ShutdownTimeout)
			defer cancel()

			lg.Info("Shutting down server", zap.Duration("timeout", cfg.Graceful.ShutdownTimeout))
			if err := server.Shutdown(shutdownCtx); err != nil {
				lg.Error("Server shutdown error", zap.Error(err))
			}
			healthSvc.Stop()
			close(shutdownDone)
		}()

		lg.Info("Server listening", zap.String("addr", cfg.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return errors.Wrap(err, "server")
		}
		<-shutdownDone
		return nil
	})
}

// apiKeyAdapter adapts postgres.APIKeyRepository to the api.APIKeyRepository
// interface, converting postgres.APIKeyRow to api.APIKeyInfo.
type apiKeyAdapter struct {
	repo *postgres.APIKeyRepository
}

func (a *apiKeyAdapter) FindByHash(ctx context.Context, hash string) (*api.APIKeyInfo, error) {
	row, err := a.repo.FindByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	return &api.APIKeyInfo{
		ID:      row.ID,
		KeyHash: row.KeyHash,
		Name:    row.Name,
		Scopes:  row.Scopes,
	}, nil
}
