# Oolio Kart -- Food Ordering Backend

[![CI](https://github.com/xenking/oolio-kart-challenge/actions/workflows/ci.yml/badge.svg)](https://github.com/xenking/oolio-kart-challenge/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/xenking/oolio-kart-challenge/branch/main/graph/badge.svg)](https://codecov.io/gh/xenking/oolio-kart-challenge)

Go backend for the Oolio Kart food ordering challenge. Product catalog, order placement with coupon discounts, API key auth, and a data pipeline that finds 8 valid promo codes in ~313 million coupon entries.

For design decisions, data pipeline details, and developer guides, see [ARCHITECTURE.md](./ARCHITECTURE.md).

## Quick Start

```bash
make up
# wait ~15s for postgres + api + seed
curl http://localhost:8080/api/product | head -c 200
```

That's it. `make up` builds the Docker image, starts Postgres, the API, seeds products/coupons/API key, and brings up the observability stack (Prometheus, Tempo, Pyroscope, Grafana).

To place an order:

```bash
curl -X POST http://localhost:8080/api/order \
  -H "Content-Type: application/json" \
  -H "api_key: dev-api-key-not-for-production" \
  -d '{
    "items": [
      {"productId": "1", "quantity": 2},
      {"productId": "2", "quantity": 1}
    ],
    "couponCode": "HAPPYHOURS"
  }'
```

To stop: `make down`.

### Local Development

```bash
docker compose up -d postgres
export DATABASE_URL="postgres://kart:kart@localhost:5432/kart?sslmode=disable"
export KART_SEED_API_KEY="my-secret-key"
export KART_API_KEY_PEPPER="my-hmac-pepper"
make seed-db
go run ./cmd/api-server
```

## Architecture

```
            Request
               │
     ┌─────────▼──────────┐
     │   ogen Router      │  ← generated from OpenAPI 3.1 spec
     │   + middleware     │    (CORS, rate limit, request ID, tracing)
     └─────────┬──────────┘
               │
     ┌─────────▼──────────┐
     │   handler layer    │  ← converts OAS types ↔ domain types
     └─────────┬──────────┘
               │
     ┌─────────▼──────────┐
     │   domain layer     │  ← business logic, zero external imports
     │   product/order/   │
     │   coupon/auth      │
     └─────────┬──────────┘
               │
     ┌─────────▼──────────┐
     │   repository       │  ← plain pgx, SQL constants, direct scanning
     └─────────┬──────────┘
               │
          PostgreSQL
```

The dependency arrow always points inward: repository imports domain, never the reverse. Handlers are thin translation layers. Business rules live in `internal/domain/` where they can be tested without HTTP or database concerns.

### Why plain pgx instead of sqlc

Nine queries. The overhead of a codegen step and double-mapping (pgx row → sqlc struct → domain struct) wasn't paying for itself. With plain pgx we scan directly into domain types, SQL lives next to the code that uses it, and there's one less tool in the build chain.

### Why ogen stays

The HTTP layer is a different story. ogen generates the router, request/response codecs, validation, and OpenTelemetry instrumentation from the OpenAPI spec. That's real leverage -- removing it would mean hand-writing hundreds of lines of boilerplate and keeping the spec in sync manually.

## API

Defined in [`api/openapi.yaml`](./api/openapi.yaml). All endpoints under `/api`.

| Method | Path                   | Auth | Description              |
|--------|------------------------|------|--------------------------|
| GET    | `/api/product`         | No   | List all products        |
| GET    | `/api/product/{id}`    | No   | Get product by ID        |
| POST   | `/api/order`           | Yes  | Place an order           |

Authentication: send the raw key in the `api_key` header. The server computes `HMAC-SHA256(pepper, key)` and does a constant-time lookup against stored hashes. If the database leaks without the pepper, key hashes can't be reversed.

Error responses: `400` for empty items, `401` for bad/missing key, `422` for invalid product, quantity, or coupon.

## Coupon System

Three discount strategies, all computed with `shopspring/decimal`:

| Type           | Behavior                                           |
|----------------|----------------------------------------------------|
| `percentage`   | N% off the subtotal                                |
| `fixed`        | Flat amount off, capped at the subtotal            |
| `free_lowest`  | Removes the cheapest item's price from the cart    |

### Extensibility

Coupons support temporal validity and usage limits without schema changes:

| Column        | Type           | Default | Meaning                           |
|---------------|----------------|---------|-----------------------------------|
| `valid_from`  | `TIMESTAMPTZ`  | NULL    | NULL = always valid from the start |
| `valid_until` | `TIMESTAMPTZ`  | NULL    | NULL = never expires              |
| `max_uses`    | `INTEGER`      | 0       | 0 = unlimited                     |
| `uses`        | `INTEGER`      | 0       | Current redemption count          |
| `max_discount`| `NUMERIC(10,2)`| 0       | 0 = no cap; otherwise clamps discount |

The validator checks these in order: temporal window, usage limit, min items, discount calculation, max discount cap, then increments the usage counter.

To add a new constraint (e.g., per-user limits, product category restrictions), add a field to `coupon.Rule` and a check in `validator.go`. No interface changes needed.

### Seeded Coupons

| Code         | Type          | Value | Min Items | Description                        |
|--------------|---------------|-------|-----------|------------------------------------|
| `HAPPYHOURS` | percentage    | 18    | 0         | Happy Hours: 18% off entire order  |
| `BUYGETONE`  | free_lowest   | 0     | 2         | Buy one get one: lowest item free  |

### Data-Derived Coupons

Eight codes found in 2+ files out of ~313 million, ingested by `cmd/coupon-ingest` using a 2-pass bloom filter algorithm (~510MB memory, parallel decompression with pgzip):

| Code       | Type        | Value | Min Items | Description                    |
|------------|-------------|-------|-----------|--------------------------------|
| `BIRTHDAY` | free_lowest | 0     | 0         | Birthday: free lowest item     |
| `BUYGETON` | free_lowest | 0     | 2         | Lowest item free (buy 2+)      |
| `FIFTYOFF` | percentage  | 50    | 0         | 50% off entire order           |
| `SIXTYOFF` | percentage  | 60    | 0         | 60% off entire order           |
| `FREEZAAA` | percentage  | 100   | 0         | Everything free!               |
| `GNULINUX` | percentage  | 15    | 0         | Open source discount: 15% off  |
| `OVER9000` | fixed       | 9     | 0         | $9 off your order              |
| `HAPPYHRS` | percentage  | 18    | 0         | Happy Hours: 18% off           |

## What Changes at Scale

This is a take-home assignment, not a production system. Here's what I'd change with real traffic:

**Coupon usage counting** -- The current `UPDATE uses = uses + 1` is a write hotspot. At scale: move to Redis atomic counters with periodic DB sync, or use a separate `coupon_redemptions` table with `SELECT COUNT(*)` and an index.

**Product catalog** -- Currently fits in a single query. At hundreds of thousands of products: add pagination, materialized views for categories, and Redis caching with invalidation on writes.

**Order placement** -- The happy path is a single INSERT. At high throughput: outbox pattern with async processing, idempotency keys to handle retries, and event sourcing if order state becomes complex.

**Rate limiting** -- In-memory sliding window works for a single instance. Behind a load balancer: Redis-backed rate limiter with the same sliding window algorithm.

**API key auth** -- HMAC hashing per request is fast enough. At millions of requests: add a short-TTL cache (30s) keyed by hash to avoid DB round-trips on every call.

## Configuration

Via `cristalhq/aconfig` with priority: flags > env vars > config files > defaults.

| Variable                         | Default        | Description                          |
|----------------------------------|----------------|--------------------------------------|
| `KART_DATABASE_URL`              | *(required)*   | PostgreSQL connection URL            |
| `KART_API_KEY_PEPPER`            | *(empty)*      | HMAC pepper for API key hashing      |
| `KART_ADDR`                      | `0.0.0.0:8080` | Listen address                       |
| `KART_IMAGE_BASE_URL`            | *(empty)*      | Prefix for product image paths       |
| `KART_RATE_LIMIT_MAX`            | `100`          | Requests per window                  |
| `KART_RATE_LIMIT_WINDOW`         | `1m`           | Rate limit window                    |
| `KART_CORS_ORIGINS`              | `*`            | Allowed CORS origins                 |
| `KART_GRACEFUL_READINESS_DELAY`  | `3s`           | Drain delay before shutdown          |
| `KART_GRACEFUL_SHUTDOWN_TIMEOUT` | `15s`          | Max graceful shutdown duration       |

## Observability

All push-based, no scraping required:

- **Grafana** at [localhost:3000](http://localhost:3000) (anonymous admin)
- **Prometheus** receives metrics via OTLP push
- **Tempo** receives traces via OTLP/gRPC
- **Pyroscope** receives CPU/memory/goroutine/mutex profiles

Health probes: `/livez` (goroutine count < 10k), `/readyz` (Postgres ping + manual ready flag). The readiness probe is used in the graceful shutdown sequence to drain connections before stopping.

## Project Structure

```
api/openapi.yaml                   OpenAPI 3.1 spec (ogen codegen source)
gen/oas/                           Generated HTTP layer (do not edit)

db/
  migrations/001_schema.sql        DDL: products, coupons, orders, api_keys
  seed/products.json               Product catalog seed data
  embed.go                         Exports db.Schema via //go:embed

cmd/
  api-server/                      Entry point: LoadConfig → app.Run
  seed-db/                         Seeds products, coupons, API key
  coupon-ingest/                   Bloom filter pipeline for .gz files

internal/
  app/                             Config + wiring (app.Run)
  domain/
    product/                       Product, Image, Repository
    order/                         Order, Service (PlaceOrder)
    coupon/                        Rule, Discount, Validator, Repository
    auth/                          APIKeyInfo, Repository
  handler/                         OAS ↔ domain conversion
  repository/                      pgx repositories (SQL constants inline)

pkg/
  health/                          K8s-style liveness/readiness probes
  httpmiddleware/                  CORS, rate limiting, request ID, recovery

deploy/                            Prometheus, Tempo, Grafana configs
tests/integration/                 Docker-based integration tests
```

## Commands

```bash
make generate        # Run ogen code generation
make build           # Build all binaries
make test            # Unit tests with race detector + coverage
make test-integration # Integration tests (requires Docker)
make lint            # Run golangci-lint
make seed-db         # Migrate + seed products, coupons, API key
make ingest-coupons  # Download gz files + bloom filter ingestion
make up              # Full stack: postgres + api + seed + observability
make down            # Stop everything
```

## License

[MIT](./LICENSE)
