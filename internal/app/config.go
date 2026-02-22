package app

import (
	"os"
	"time"

	"github.com/cristalhq/aconfig"
	"github.com/cristalhq/aconfig/aconfigyaml"
	"github.com/go-faster/errors"
)

// Config holds the complete application configuration, loadable from
// environment variables (KART_ prefix), flags, or YAML config files.
type Config struct {
	Addr         string `default:"0.0.0.0:8080" usage:"API server listen address"`
	DatabaseURL  string `usage:"PostgreSQL connection URL (KART_DATABASE_URL or DATABASE_URL)" flag:"database-url"`
	ImageBaseURL string `default:"" usage:"Base URL for product images (e.g. https://cdn.example.com/images)" flag:"image-base-url"`
	APIKeyPepper string `usage:"HMAC pepper for API key hashing (KART_API_KEY_PEPPER)" flag:"api-key-pepper"`
	RateLimit    RateLimitConfig
	CORS         CORSConfig
	Graceful     GracefulConfig
}

// RateLimitConfig controls the per-client sliding window rate limiter.
type RateLimitConfig struct {
	Max    int           `default:"100" usage:"Max requests per window"`
	Window time.Duration `default:"1m"  usage:"Rate limit window duration"`
}

// CORSConfig controls Cross-Origin Resource Sharing headers.
type CORSConfig struct {
	Origins          []string `default:"*" usage:"Allowed CORS origins"`
	AllowCredentials bool     `default:"false" usage:"Allow credentials (cookies, auth headers)" flag:"cors-credentials"`
}

// GracefulConfig controls graceful shutdown timing.
type GracefulConfig struct {
	ReadinessDelay  time.Duration `default:"3s"  usage:"Delay after readiness=false before shutdown" flag:"readiness-delay"`
	ShutdownTimeout time.Duration `default:"15s" usage:"Maximum shutdown duration" flag:"shutdown-timeout"`
}

// LoadConfig loads configuration from environment variables, YAML config files,
// and applies platform-specific defaults.
func LoadConfig() (*Config, error) {
	var cfg Config
	loader := aconfig.LoaderFor(&cfg, aconfig.Config{
		EnvPrefix: "KART",
		Files:     []string{"config.yaml", "/etc/kart/config.yaml"},
		FileDecoders: map[string]aconfig.FileDecoder{
			".yaml": aconfigyaml.New(),
		},
	})
	if err := loader.Load(); err != nil {
		return nil, errors.Wrap(err, "load config")
	}
	cfg.applyPlatformDefaults()

	if cfg.DatabaseURL == "" {
		return nil, errors.New("database URL is required: set KART_DATABASE_URL or DATABASE_URL")
	}

	return &cfg, nil
}

// applyPlatformDefaults maps platform-provided environment variables (Railway,
// Render, etc.) that use standard names like DATABASE_URL and PORT to the
// application's KART_-prefixed configuration.
func (c *Config) applyPlatformDefaults() {
	if c.DatabaseURL == "" {
		if v := os.Getenv("DATABASE_URL"); v != "" {
			c.DatabaseURL = v
		}
	}
	if port := os.Getenv("PORT"); port != "" && c.Addr == "0.0.0.0:8080" {
		c.Addr = "0.0.0.0:" + port
	}
}
