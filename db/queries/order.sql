-- name: CreateOrder :exec
INSERT INTO orders (id, items, total, discounts, coupon_code)
VALUES ($1, $2, $3, $4, $5);

-- name: GetOrderByID :one
SELECT * FROM orders WHERE id = $1;
