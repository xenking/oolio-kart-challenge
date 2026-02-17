package coupon

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/shopspring/decimal"
)

// DiscountType enumerates the supported coupon discount strategies.
type DiscountType string

const (
	// DiscountPercentage applies a percentage-based discount to the subtotal.
	DiscountPercentage DiscountType = "percentage"
	// DiscountFixed applies a fixed monetary discount capped at the subtotal.
	DiscountFixed DiscountType = "fixed"
	// DiscountFreeLowest removes the cost of the cheapest item in the cart.
	DiscountFreeLowest DiscountType = "free_lowest"
)

// ErrInvalidCoupon is returned when a coupon code is not found or
// the cart does not satisfy the coupon's minimum item requirement.
var ErrInvalidCoupon = errors.New("invalid coupon code")

// Rule defines a coupon's discount behaviour and eligibility constraints.
type Rule struct {
	Code         string
	DiscountType DiscountType
	Value        decimal.Decimal
	MinItems     int
	Description  string
}

// Discount holds the computed discount amount and a human-readable description.
type Discount struct {
	Amount      decimal.Decimal
	Description string
}

// Item represents a line item in the cart for discount calculation purposes.
type Item struct {
	ProductID string
	Price     decimal.Decimal
	Quantity  int
}

// Repository provides lookup of coupon rules by their code.
type Repository interface {
	FindByCode(ctx context.Context, code string) (*Rule, error)
}
