# Oolio Kart -- Food Ordering Backend

[![CI](https://github.com/xenking/oolio-kart-challenge/actions/workflows/ci.yml/badge.svg)](https://github.com/xenking/oolio-kart-challenge/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/xenking/oolio-kart-challenge/branch/main/graph/badge.svg)](https://codecov.io/gh/xenking/oolio-kart-challenge)

A production-ready Go backend for the Oolio Kart food ordering challenge. It provides a product catalog API, order placement with coupon validation, API key authentication, and a data engineering pipeline that processes ~313 million coupon codes to extract 8 valid promo codes.

## Table of Contents

- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [API Reference](#api-reference)
- [Coupon System](#coupon-system)
- [Data Ingestion Pipeline](#data-ingestion-pipeline)
- [Configuration](#configuration)
- [Observability](#observability)
- [Project Structure](#project-structure)
- [Commands](#commands)
- [Design Decisions](#design-decisions)
- [License](#license)

## Architecture

The application runs a single HTTP server with health probes, middleware, and push-based telemetry:

```
                    :8080 (API Server)
                   ┌─────────────────┐
                   │  /livez /readyz │
                   │  CORS           │
  Clients ──────>  │  Rate Limiting  │──── OTLP push ───> Prometheus (metrics)
                   │  Request IDs    │──── OTLP push ───> Tempo (traces)
                   │  OTel Tracing   │──── Push ────────> Pyroscope (profiles)
                   │  Request Logger │
                   └────────┬────────┘
                            │
                   ┌────────▼────────┐
                   │  ogen-generated │
                   │  Router + Codec │
                   └────────┬────────┘
                            │
              ┌─────────────┼──────────────┐
              │             │              │
     ┌────────▼──┐  ┌───────▼───┐  ┌──────▼─────┐
     │  Product  │  │   Order   │  │  Security  │
     │  Handler  │  │  Handler  │  │  Handler   │
     └─────┬─────┘  └─────┬─────┘  └──────┬─────┘
           │               │               │
     ┌─────▼─────┐  ┌─────▼─────┐  ┌──────▼─────┐
     │  product  │  │   order   │  │   coupon   │
     │   .Repo   │  │   .Repo   │  │ .Validator │
     └─────┬─────┘  └─────┬─────┘  └──────┬─────┘
           │               │               │
           └───────────────┼───────────────┘
                           │
                  ┌────────▼────────┐
                  │   PostgreSQL    │
                  │  (pgx/v5 pool) │
                  └─────────────────┘
```

Separating public API traffic from internal operational endpoints means health checks and Prometheus scraping are never rate-limited, and the internal port can be firewalled in production.

## Prerequisites

- **Go 1.25+**
- **Docker** and **Docker Compose** (for PostgreSQL and observability stack)
- **Make** (optional, for convenience targets)
- **curl** (for downloading coupon data files)

## Quick Start

### Option A: Docker Compose (recommended)

Start the full stack including PostgreSQL, the API, Prometheus, Tempo, and Grafana:

```bash
make up
```

This builds the Docker image, runs migrations, and starts all services. The API is available at `http://localhost:8080`.

To stop everything:

```bash
make down
```

### Option B: Local development

Start only the infrastructure:

```bash
docker compose up -d postgres prometheus tempo grafana
```

Seed the database with products, challenge coupons, and the default API key:

```bash
make seed-db
```

Optionally, download and ingest the data-derived coupon codes:

```bash
make ingest-coupons
```

Run the API server:

```bash
export KART_DATABASE_URL="postgres://kart:kart@localhost:5432/kart?sslmode=disable"
go run ./cmd/api-server
```

### Verify it works

```bash
# List all products
curl -s http://localhost:8080/api/product | head -c 200

# Get a single product
curl -s http://localhost:8080/api/product/1

# Place an order with a coupon (requires API key)
curl -s -X POST http://localhost:8080/api/order \
  -H "Content-Type: application/json" \
  -H "api_key: apitest" \
  -d '{
    "items": [
      {"productId": "1", "quantity": 2},
      {"productId": "2", "quantity": 1}
    ],
    "couponCode": "HAPPYHOURS"
  }'
```

## API Reference

The API is defined by an [OpenAPI 3.1 specification](./_oas/openapi.yaml). All endpoints are served under the `/api` prefix.

### List Products

```
GET /api/product
```

Returns an array of all products in the catalog. No authentication required.

**Response** `200 OK`:

```json
[
  {
    "id": "1",
    "name": "Waffle with Berries",
    "price": 6.5,
    "category": "Waffle",
    "image": {
      "thumbnail": "https://orderfoodonline.deno.dev/public/images/image-waffle-thumbnail.jpg",
      "mobile": "https://orderfoodonline.deno.dev/public/images/image-waffle-mobile.jpg",
      "tablet": "https://orderfoodonline.deno.dev/public/images/image-waffle-tablet.jpg",
      "desktop": "https://orderfoodonline.deno.dev/public/images/image-waffle-desktop.jpg"
    }
  }
]
```

### Get Product by ID

```
GET /api/product/{productId}
```

Returns a single product. Returns `404` if the product does not exist.

### Place Order

```
POST /api/order
```

**Authentication**: Requires the `api_key` header. Use `apitest` for the default test key.

**Request body**:

```json
{
  "items": [
    { "productId": "1", "quantity": 2 },
    { "productId": "3", "quantity": 1 }
  ],
  "couponCode": "HAPPYHOURS"
}
```

| Field        | Type     | Required | Description                        |
|--------------|----------|----------|------------------------------------|
| `items`      | array    | Yes      | At least one item                  |
| `items[].productId` | string | Yes | Product ID from the catalog       |
| `items[].quantity`   | int    | Yes | Must be >= 1                      |
| `couponCode` | string   | No       | Optional promo code for a discount |

**Response** `200 OK`:

```json
{
  "id": "a1b2c3d4-...",
  "total": 10.66,
  "discounts": 2.34,
  "items": [
    { "productId": "1", "quantity": 2 },
    { "productId": "3", "quantity": 1 }
  ],
  "products": [ ... ]
}
```

**Error responses**:

| Status | Condition                                |
|--------|------------------------------------------|
| `400`  | Empty items array                        |
| `401`  | Missing or invalid API key               |
| `422`  | Invalid product ID, quantity, or coupon  |

### Authentication

API key authentication uses SHA-256 hashing with constant-time comparison:

1. The client sends the raw key in the `api_key` header.
2. The server hashes it with SHA-256 and looks up the hash in the `api_keys` table.
3. A constant-time comparison (`crypto/subtle.ConstantTimeCompare`) guards against timing side-channels.

The default test key `apitest` is seeded by `cmd/seed-db`.

### Rate Limiting

All API server responses include rate limit headers:

| Header                  | Description                           |
|-------------------------|---------------------------------------|
| `X-RateLimit-Limit`     | Maximum requests per window (default: 100) |
| `X-RateLimit-Remaining` | Remaining requests in current window  |
| `X-RateLimit-Reset`     | Unix timestamp when the window resets |
| `Retry-After`           | Seconds to wait (only on `429`)       |

The rate limiter uses a sliding window algorithm keyed by client IP (extracted from `X-Forwarded-For`, `X-Real-IP`, or `RemoteAddr`).

## Coupon System

### Discount Types

The coupon engine supports three discount strategies, all computed with `shopspring/decimal` to avoid floating-point rounding errors:

| Type           | Behavior                                              |
|----------------|-------------------------------------------------------|
| `percentage`   | Applies N% off the subtotal                           |
| `fixed`        | Subtracts a fixed amount, capped at the subtotal      |
| `free_lowest`  | Removes the cost of the cheapest item in the cart     |

Discounts are floored at zero (an order total can never go negative) and rounded to 2 decimal places.

### Challenge Coupons (seeded)

These are seeded by `cmd/seed-db` and satisfy the challenge requirements:

| Code         | Type          | Value | Min Items | Description                         |
|--------------|---------------|-------|-----------|-------------------------------------|
| `HAPPYHOURS` | percentage    | 18    | 0         | Happy Hours: 18% off entire order   |
| `BUYGETONE`  | free_lowest   | 0     | 2         | Buy one get one: lowest item free   |

### Data-Derived Coupons (from gz files)

Eight codes found in 2+ files out of ~313 million total, ingested by `cmd/coupon-ingest`:

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

## Data Ingestion Pipeline

The `cmd/coupon-ingest` tool processes three gzip-compressed files (each ~100M+ codes) to find coupon codes that appear in two or more files.

### Algorithm: 2-Pass Bloom Filter

**Pass 1 -- Build bloom filters (concurrent):**

For each of the 3 files, a bloom filter is built in parallel. Each filter holds ~120M entries at a 0.1% false positive rate, consuming approximately 170MB of memory.

**Pass 2 -- Find candidates (concurrent):**

Each file is streamed again. For every code, the pipeline checks whether it exists in any *other* file's bloom filter. Candidates are tracked with a bitmask indicating which files contain them.

**Validation:**

After merging bitmasks from all files, codes with `popcount(bitmask) >= 2` are confirmed valid.

### Performance Characteristics

| Metric         | Value                                     |
|----------------|-------------------------------------------|
| Total codes    | ~313 million across 3 files               |
| Memory         | ~510MB for bloom filters                  |
| Decompression  | `klauspost/pgzip` for parallel gunzip     |
| Concurrency    | Files processed in parallel via `errgroup` |
| Code filtering | Only codes 8-10 chars are considered      |

### Code Length Distribution

Analysis of the input files reveals that valid codes are exactly 8 characters, while the vast majority of noise codes are 9 or 10 characters:

- **File 1**: 107,260,777 codes, all 8 chars
- **File 2**: 107,260,768 codes at 9 chars + 8 codes at 8 chars
- **File 3**: 98,566,144 codes at 10 chars + 8 codes at 8 chars

## Configuration

Configuration is loaded via `cristalhq/aconfig` with the following priority (highest to lowest):

1. Command-line flags
2. Environment variables (`KART_` prefix)
3. Config files (`config.yaml`, `/etc/kart/config.yaml`)
4. Default values

### Environment Variables

| Variable              | Default         | Description                          |
|-----------------------|-----------------|--------------------------------------|
| `KART_DATABASE_URL`   | *(required)*    | PostgreSQL connection URL            |
| `KART_ADDR`           | `0.0.0.0:8080`  | API server listen address            |
| `KART_IMAGE_BASE_URL` | *(empty)*       | Base URL for product images          |
| `KART_RATE_LIMIT_MAX` | `100`           | Max requests per rate limit window   |
| `KART_RATE_LIMIT_WINDOW` | `1m`         | Rate limit window duration           |
| `KART_CORS_ORIGINS`   | `*`             | Allowed CORS origins                 |
| `KART_GRACEFUL_READINESS_DELAY` | `3s` | Delay after readiness=false before shutdown |
| `KART_GRACEFUL_SHUTDOWN_TIMEOUT` | `15s` | Maximum graceful shutdown duration |

### Default config.yaml

```yaml
addr: "0.0.0.0:8080"
image_base_url: "https://orderfoodonline.deno.dev/public"
rate_limit:
  max: 100
  window: 1m
cors:
  origins: ["*"]
graceful:
  readiness_delay: 3s
  shutdown_timeout: 15s
```

## Observability

### Grafana

Available at [http://localhost:3000](http://localhost:3000) with anonymous admin access. Pre-configured datasources for Prometheus, Tempo, and Pyroscope.

### Prometheus

Available at [http://localhost:9090](http://localhost:9090). Receives metrics via OTLP push from the API server (no scraping needed).

### Distributed Tracing

Tempo collects traces via OTLP/gRPC on port 4317. The API server instruments all requests with OpenTelemetry spans including operation names from the ogen router.

### Continuous Profiling

Pyroscope receives push-based profiles (CPU, memory, goroutines, mutex) from the API server via `go-faster/sdk/autopyro`. Available at [http://localhost:4040](http://localhost:4040).

### Health Checks

The `pkg/health` package provides Kubernetes-style probes with failure/success thresholds to prevent flapping:

| Endpoint  | Type       | Checks                                 |
|-----------|------------|----------------------------------------|
| `/livez`  | Liveness   | Goroutine count < 10,000               |
| `/readyz` | Readiness  | PostgreSQL connectivity + manual ready flag |

The readiness probe is used in the graceful shutdown sequence: readiness is set to `false`, the server waits for the configured delay (allowing load balancers to drain), then shuts down.

## Project Structure

```
_oas/openapi.yaml              OpenAPI 3.1 specification
cmd/
  api-server/                  Main API server entry point
    main.go                    Server setup, wiring, graceful shutdown
    config.go                  Configuration struct with aconfig tags
  seed-db/                     Database seeding (products, coupons, API key)
  coupon-ingest/               Bloom filter coupon ingestion pipeline
internal/
  api/                         HTTP handlers and security
    handler.go                 Handler struct, dependency injection
    product.go                 ListProducts, GetProduct handlers
    order.go                   PlaceOrder handler with coupon + decimal math
    security.go                SHA-256 API key authentication
  coupon/                      Coupon domain
    coupon.go                  Types: Rule, Discount, Item, DiscountType
    discount.go                Apply logic: percentage, fixed, free_lowest
    validator.go               RepoValidator wrapping Repository + Apply
  order/                       Order domain types
  product/                     Product domain types
  postgres/                    PostgreSQL repository implementations
    postgres.go                Pool creation, embedded migrations
    product.go                 product.Repository implementation
    coupon.go                  coupon.Repository implementation
    order.go                   order.Repository implementation
    apikey.go                  API key lookup by SHA-256 hash
  oas/                         Generated ogen code (do not edit)
  dbgen/                       Generated sqlc code (do not edit)
pkg/
  httpmiddleware/              HTTP middleware stack
    httpmiddleware.go          Wrap, InjectLogger, Labeler, Instrument, LogRequests
    cors.go                    CORS middleware
    ratelimit.go               Sliding window rate limiter
    requestid.go               X-Request-ID injection
    ogen.go                    ogen RouteFinder adapter
  health/                      Kubernetes-style health check package
    health.go                  Health service with background check runners
    checkers.go                Built-in check functions (goroutine count, etc.)
db/
  migrations/001_schema.sql    DDL: products, coupons, orders, api_keys tables
  queries/                     sqlc SQL queries (product, coupon, order, apikey)
  seed/products.json           Product catalog seed data
deploy/                        Observability configs
  prometheus.yml               Prometheus scrape configuration
  tempo.yml                    Tempo trace backend configuration
  grafana/datasources.yml      Grafana datasource provisioning
scripts/
  download-coupons.sh          Downloads couponbase{1,2,3}.gz from S3
config.yaml                    Default application configuration
docker-compose.yml             Full stack: PostgreSQL, API, Prometheus, Tempo, Grafana
Dockerfile                     Multi-stage build (Go 1.25 alpine)
Makefile                       Build, test, lint, seed, ingest targets
sqlc.yaml                      sqlc code generation configuration
gen.go                         go:generate directive for ogen
```

## Commands

```bash
make generate          # Run ogen + sqlc code generation
make build             # Build all binaries (api-server, seed-db, coupon-ingest)
make test              # Run all tests
make lint              # Run golangci-lint
make seed-db           # Run migrations + seed products, coupons, and API key
make download-coupons  # Download couponbase{1,2,3}.gz from S3
make ingest-coupons    # Download gz files + run bloom filter ingestion
make up                # Docker compose up --build -d (full stack)
make down              # Docker compose down
```

## Design Decisions

### PostgreSQL over SQLite

PostgreSQL supports concurrent writes, NUMERIC types for precise decimal math, and shared state for horizontal scaling. The `pgx/v5` driver with `pgxpool` provides connection pooling suitable for production workloads.

### shopspring/decimal for monetary math

All pricing and discount calculations use `decimal.Decimal` instead of `float64`. This eliminates rounding errors that are common with IEEE 754 floating-point arithmetic in financial contexts. Values are stored as `NUMERIC(10,2)` in PostgreSQL.

### 2-pass bloom filter for coupon ingestion

Instead of loading all ~313M codes into memory (which would require tens of gigabytes), the pipeline uses bloom filters at ~170MB each. Two passes over the data identify codes appearing in 2+ files with high confidence. The 0.1% false positive rate is acceptable because the candidate set is tiny (8 codes out of 313M).

### Repository pattern

Domain types (`product.Product`, `order.Order`, `coupon.Rule`) and repository interfaces live in their own packages, decoupled from the PostgreSQL implementation. This keeps the API handlers testable without a database.

### Single server with push telemetry

A single HTTP server on `:8080` serves both the API and health probes (`/livez`, `/readyz`). Metrics, traces, and profiles are pushed to their respective backends (Prometheus via OTLP, Tempo via OTLP, Pyroscope) rather than pulled, eliminating the need for a second internal server.

### ogen + sqlc code generation

Both the HTTP layer (ogen from OpenAPI 3.1) and the database layer (sqlc from SQL queries) use code generation. This provides compile-time type safety, eliminates hand-written boilerplate, and keeps the API contract in sync with the spec.

### Graceful shutdown sequence

The shutdown follows a Kubernetes-friendly pattern:
1. Readiness probe returns unhealthy (`SetReady(false)`)
2. Wait for configured delay (default 3s) to allow load balancers to drain
3. Shut down the HTTP server with a timeout (default 15s)
4. Stop health check background goroutines

## Database Schema

Four tables manage the application state:

```sql
products    -- Product catalog (id, name, price, category, images)
coupons     -- Coupon rules (code, discount_type, value, min_items, active)
orders      -- Placed orders (id, items as JSONB, total, discounts, coupon_code)
api_keys    -- API authentication (id, key_hash, name, scopes, active)
```

Migrations are embedded in the Go binary via `//go:embed` and run automatically on startup.

## Technology Stack

| Component         | Technology                                       |
|-------------------|--------------------------------------------------|
| Language          | Go 1.25+                                        |
| HTTP Framework    | ogen (OpenAPI 3.1 code generation)               |
| Database          | PostgreSQL 17, pgx/v5, pgxpool                   |
| SQL Codegen       | sqlc                                             |
| Decimal Math      | shopspring/decimal                               |
| App Lifecycle     | go-faster/sdk (telemetry, logging, graceful)     |
| Observability     | OpenTelemetry, Prometheus, Grafana, Tempo         |
| Rate Limiting     | Custom sliding window (per-IP)                   |
| Bloom Filters     | bits-and-blooms/bloom                            |
| Compression       | klauspost/pgzip (parallel decompression)         |
| Configuration     | cristalhq/aconfig + YAML                         |
| Logging           | uber-go/zap (structured)                         |
| Containerization  | Multi-stage Docker build, Docker Compose         |

## License

[MIT](./LICENSE)
