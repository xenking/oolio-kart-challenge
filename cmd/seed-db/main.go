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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/xenking/oolio-kart-challenge/internal/repository"
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

	pool, err := repository.NewPool(ctx, databaseURL)
	if err != nil {
		return errors.Wrap(err, "connect to database")
	}
	defer pool.Close()

	slog.Info("running migrations")

	if err := repository.RunMigrations(ctx, pool); err != nil {
		return errors.Wrap(err, "run migrations")
	}

	if err := seedProducts(ctx, pool, productsFile); err != nil {
		return errors.Wrap(err, "seed products")
	}

	if err := seedCoupons(ctx, pool); err != nil {
		return errors.Wrap(err, "seed coupons")
	}

	if err := seedAPIKey(ctx, pool, apiKey, pepper); err != nil {
		return errors.Wrap(err, "seed api key")
	}

	return nil
}

const upsertProductSQL = `INSERT INTO products (id, name, price, category, image_thumbnail, image_mobile, image_tablet, image_desktop)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	ON CONFLICT (id) DO UPDATE SET
		name = EXCLUDED.name, price = EXCLUDED.price, category = EXCLUDED.category,
		image_thumbnail = EXCLUDED.image_thumbnail, image_mobile = EXCLUDED.image_mobile,
		image_tablet = EXCLUDED.image_tablet, image_desktop = EXCLUDED.image_desktop`

func seedProducts(ctx context.Context, pool *pgxpool.Pool, productsFile string) error {
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
		if _, err := pool.Exec(ctx, upsertProductSQL,
			p.ID, p.Name, p.Price, p.Category,
			p.Image.Thumbnail, p.Image.Mobile, p.Image.Tablet, p.Image.Desktop,
		); err != nil {
			return errors.Wrapf(err, "upsert product %s", p.ID)
		}

		slog.Info("upserted product", slog.String("id", p.ID), slog.String("name", p.Name))
	}

	return nil
}

const upsertCouponSQL = `INSERT INTO coupons (code, discount_type, value, min_items, description, active)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (code) DO UPDATE SET
		discount_type = EXCLUDED.discount_type, value = EXCLUDED.value,
		min_items = EXCLUDED.min_items, description = EXCLUDED.description,
		active = EXCLUDED.active`

type seedCoupon struct {
	code         string
	discountType string
	value        decimal.Decimal
	minItems     int
	description  string
}

func seedCoupons(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("seeding challenge coupons")

	coupons := []seedCoupon{
		{
			code:         "HAPPYHOURS",
			discountType: "percentage",
			value:        decimal.NewFromInt(18),
			minItems:     0,
			description:  "Happy Hours: 18% off entire order",
		},
		{
			code:         "BUYGETONE",
			discountType: "free_lowest",
			value:        decimal.Zero,
			minItems:     2,
			description:  "Buy one get one: lowest priced item free",
		},
	}

	for _, c := range coupons {
		if _, err := pool.Exec(ctx, upsertCouponSQL,
			c.code, c.discountType, c.value, c.minItems, c.description, true,
		); err != nil {
			return errors.Wrapf(err, "upsert coupon %s", c.code)
		}

		slog.Info("upserted coupon", slog.String("code", c.code), slog.String("description", c.description))
	}

	return nil
}

const upsertAPIKeySQL = `INSERT INTO api_keys (id, key_hash, name, scopes, active)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (id) DO UPDATE SET
		key_hash = EXCLUDED.key_hash, name = EXCLUDED.name,
		scopes = EXCLUDED.scopes, active = EXCLUDED.active`

func seedAPIKey(ctx context.Context, pool *pgxpool.Pool, apiKey, pepper string) error {
	slog.Info("seeding default API key")

	mac := hmac.New(sha256.New, []byte(pepper))
	mac.Write([]byte(apiKey))
	keyHash := hex.EncodeToString(mac.Sum(nil))

	if _, err := pool.Exec(ctx, upsertAPIKeySQL,
		"default", keyHash, "Default test key", []string{"create_order"}, true,
	); err != nil {
		return errors.Wrap(err, "upsert default API key")
	}

	slog.Info("upserted API key", slog.String("id", "default"), slog.String("name", "Default test key"))

	return nil
}
