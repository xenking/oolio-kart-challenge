package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	"github.com/go-faster/errors"
	"github.com/shopspring/decimal"

	"github.com/xenking/oolio-kart-challenge/gen/sqlc"
	"github.com/xenking/oolio-kart-challenge/internal/storage/postgres"
)

type productJSON struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Price    decimal.Decimal `json:"price"`
	Category string          `json:"category"`
	Image    struct {
		Thumbnail string `json:"thumbnail"`
		Mobile    string `json:"mobile"`
		Tablet    string `json:"tablet"`
		Desktop   string `json:"desktop"`
	} `json:"image"`
}

func main() {
	var (
		databaseURL  string
		productsFile string
		apiKey       string
		apiKeyPepper string
	)

	flag.StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL (or DATABASE_URL env)")
	flag.StringVar(&productsFile, "products-file", "db/seed/products.json", "path to products JSON file")
	flag.StringVar(&apiKey, "api-key", "", "API key to seed (or KART_SEED_API_KEY env)")
	flag.StringVar(&apiKeyPepper, "api-key-pepper", "", "HMAC pepper for API key hashing (or KART_API_KEY_PEPPER env)")
	flag.Parse()

	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		slog.Error("database URL is required: set --database-url or DATABASE_URL")
		os.Exit(1)
	}
	if apiKey == "" {
		apiKey = os.Getenv("KART_SEED_API_KEY")
	}
	if apiKey == "" {
		slog.Error("API key is required: set --api-key or KART_SEED_API_KEY")
		os.Exit(1)
	}
	if apiKeyPepper == "" {
		apiKeyPepper = os.Getenv("KART_API_KEY_PEPPER")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, databaseURL, productsFile, apiKey, apiKeyPepper); err != nil {
		slog.Error("seed failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("seed completed successfully")
}

func run(ctx context.Context, databaseURL, productsFile, apiKey, pepper string) error {
	slog.Info("connecting to database")

	pool, err := postgres.NewPool(ctx, databaseURL)
	if err != nil {
		return errors.Wrap(err, "connect to database")
	}
	defer pool.Close()

	slog.Info("running migrations")

	if err := postgres.RunMigrations(ctx, pool); err != nil {
		return errors.Wrap(err, "run migrations")
	}

	queries := sqlc.New(pool)

	if err := seedProducts(ctx, queries, productsFile); err != nil {
		return errors.Wrap(err, "seed products")
	}

	if err := seedCoupons(ctx, queries); err != nil {
		return errors.Wrap(err, "seed coupons")
	}

	if err := seedAPIKey(ctx, queries, apiKey, pepper); err != nil {
		return errors.Wrap(err, "seed api key")
	}

	return nil
}

func seedProducts(ctx context.Context, queries *sqlc.Queries, productsFile string) error {
	slog.Info("reading products file", slog.String("path", productsFile))

	data, err := os.ReadFile(productsFile)
	if err != nil {
		return errors.Wrap(err, "read products file")
	}

	var products []productJSON
	if err := json.Unmarshal(data, &products); err != nil {
		return errors.Wrap(err, "parse products JSON")
	}

	slog.Info("upserting products", slog.Int("count", len(products)))

	for _, p := range products {
		if err := queries.UpsertProduct(ctx, sqlc.UpsertProductParams{
			ID:             p.ID,
			Name:           p.Name,
			Price:          p.Price,
			Category:       p.Category,
			ImageThumbnail: p.Image.Thumbnail,
			ImageMobile:    p.Image.Mobile,
			ImageTablet:    p.Image.Tablet,
			ImageDesktop:   p.Image.Desktop,
		}); err != nil {
			return errors.Wrapf(err, "upsert product %s", p.ID)
		}

		slog.Info("upserted product", slog.String("id", p.ID), slog.String("name", p.Name))
	}

	return nil
}

func seedCoupons(ctx context.Context, queries *sqlc.Queries) error {
	slog.Info("seeding challenge coupons")

	coupons := []sqlc.UpsertCouponParams{
		{
			Code:         "HAPPYHOURS",
			DiscountType: "percentage",
			Value:        decimal.NewFromInt(18),
			MinItems:     0,
			Description:  "Happy Hours: 18% off entire order",
			Active:       true,
		},
		{
			Code:         "BUYGETONE",
			DiscountType: "free_lowest",
			Value:        decimal.Zero,
			MinItems:     2,
			Description:  "Buy one get one: lowest priced item free",
			Active:       true,
		},
	}

	for _, c := range coupons {
		if err := queries.UpsertCoupon(ctx, c); err != nil {
			return errors.Wrapf(err, "upsert coupon %s", c.Code)
		}

		slog.Info("upserted coupon", slog.String("code", c.Code), slog.String("description", c.Description))
	}

	return nil
}

func seedAPIKey(ctx context.Context, queries *sqlc.Queries, apiKey, pepper string) error {
	slog.Info("seeding default API key")

	mac := hmac.New(sha256.New, []byte(pepper))
	mac.Write([]byte(apiKey))
	keyHash := hex.EncodeToString(mac.Sum(nil))

	if err := queries.UpsertAPIKey(ctx, sqlc.UpsertAPIKeyParams{
		ID:      "default",
		KeyHash: keyHash,
		Name:    "Default test key",
		Scopes:  []string{"create_order"},
		Active:  true,
	}); err != nil {
		return errors.Wrap(err, "upsert default API key")
	}

	slog.Info("upserted API key", slog.String("id", "default"), slog.String("name", "Default test key"))

	return nil
}
