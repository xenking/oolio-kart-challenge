package order

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Order represents a completed customer order with pricing and discount details.
type Order struct {
	ID         string
	Items      []OrderItem
	Total      decimal.Decimal
	Discounts  decimal.Decimal
	CouponCode string
	CreatedAt  time.Time
}

// OrderItem represents a single line item in an order.
type OrderItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// Repository defines persistence operations for orders.
type Repository interface {
	Create(ctx context.Context, order *Order) error
}
