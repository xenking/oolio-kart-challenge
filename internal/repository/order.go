package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xenking/oolio-kart-challenge/internal/domain/order"
)

const createOrderSQL = `INSERT INTO orders (id, items, total, discounts, coupon_code)
	VALUES ($1, $2, $3, $4, $5)`

var _ order.Repository = (*OrderRepository)(nil)

// OrderRepository implements order.Repository backed by PostgreSQL.
type OrderRepository struct {
	pool *pgxpool.Pool
}

// NewOrderRepository returns an OrderRepository that uses the given pool.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

// Create persists a new order. The order items are serialized to JSON for
// storage in the JSONB column.
func (r *OrderRepository) Create(ctx context.Context, o *order.Order) error {
	itemsJSON, err := json.Marshal(o.Items)
	if err != nil {
		return fmt.Errorf("marshaling order items: %w", err)
	}

	_, err = r.pool.Exec(ctx, createOrderSQL,
		o.ID, itemsJSON, o.Total, o.Discounts, o.CouponCode,
	)
	if err != nil {
		return fmt.Errorf("creating order %q: %w", o.ID, err)
	}

	return nil
}
