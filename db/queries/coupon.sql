-- name: GetCouponByCode :one
SELECT code, discount_type, value, min_items, description
FROM coupons
WHERE code = UPPER($1) AND active = TRUE;

-- name: UpsertCoupon :exec
INSERT INTO coupons (code, discount_type, value, min_items, description, active)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (code) DO UPDATE SET
    discount_type = EXCLUDED.discount_type,
    value = EXCLUDED.value,
    min_items = EXCLUDED.min_items,
    description = EXCLUDED.description,
    active = EXCLUDED.active;
