CREATE TABLE IF NOT EXISTS products (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    price           NUMERIC(10,2) NOT NULL,
    category        TEXT NOT NULL,
    image_thumbnail TEXT NOT NULL DEFAULT '',
    image_mobile    TEXT NOT NULL DEFAULT '',
    image_tablet    TEXT NOT NULL DEFAULT '',
    image_desktop   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS coupons (
    code          TEXT PRIMARY KEY,
    discount_type TEXT NOT NULL CHECK (discount_type IN ('percentage', 'fixed', 'free_lowest')),
    value         NUMERIC(10,2) NOT NULL DEFAULT 0,
    min_items     INTEGER NOT NULL DEFAULT 0,
    description   TEXT NOT NULL DEFAULT '',
    active        BOOLEAN NOT NULL DEFAULT TRUE,
    valid_from    TIMESTAMPTZ,
    valid_until   TIMESTAMPTZ,
    max_uses      INTEGER NOT NULL DEFAULT 0,
    uses          INTEGER NOT NULL DEFAULT 0,
    max_discount  NUMERIC(10,2) NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_coupons_active ON coupons(code) WHERE active = TRUE;

CREATE TABLE IF NOT EXISTS orders (
    id          TEXT PRIMARY KEY,
    items       JSONB NOT NULL,
    total       NUMERIC(10,2) NOT NULL,
    discounts   NUMERIC(10,2) NOT NULL DEFAULT 0,
    coupon_code TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id         TEXT PRIMARY KEY,
    key_hash   TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL DEFAULT '',
    scopes     TEXT[] NOT NULL DEFAULT '{}',
    active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
