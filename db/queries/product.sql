-- name: ListProducts :many
SELECT * FROM products ORDER BY id;

-- name: GetProductByID :one
SELECT * FROM products WHERE id = $1;

-- name: UpsertProduct :exec
INSERT INTO products (id, name, price, category, image_thumbnail, image_mobile, image_tablet, image_desktop)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    price = EXCLUDED.price,
    category = EXCLUDED.category,
    image_thumbnail = EXCLUDED.image_thumbnail,
    image_mobile = EXCLUDED.image_mobile,
    image_tablet = EXCLUDED.image_tablet,
    image_desktop = EXCLUDED.image_desktop;
