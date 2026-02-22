package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xenking/oolio-kart-challenge/gen/sqlc"
	"github.com/xenking/oolio-kart-challenge/internal/domain/order"
)

var _ order.Repository = (*OrderRepository)(nil)

// OrderRepository implements order.Repository backed by PostgreSQL.
type OrderRepository struct {
	q *sqlc.Queries
}

// NewOrderRepository returns an OrderRepository that uses the given pool.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{q: sqlc.New(pool)}
}

// Create persists a new order. The order items are serialized to JSON for
// storage in the JSONB column.
func (r *OrderRepository) Create(ctx context.Context, o *order.Order) error {
	itemsJSON, err := json.Marshal(o.Items)
	if err != nil {
		return fmt.Errorf("marshaling order items: %w", err)
	}

	err = r.q.CreateOrder(ctx, sqlc.CreateOrderParams{
		ID:         o.ID,
		Items:      itemsJSON,
		Total:      o.Total,
		Discounts:  o.Discounts,
		CouponCode: o.CouponCode,
	})
	if err != nil {
		return fmt.Errorf("creating order %q: %w", o.ID, err)
	}

	return nil
}
