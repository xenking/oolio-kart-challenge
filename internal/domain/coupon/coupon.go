package coupon

import (
	"context"
	"time"

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

var (
	// ErrInvalidCoupon is returned when a coupon code is not found or
	// the cart does not satisfy the coupon's minimum item requirement.
	ErrInvalidCoupon = errors.New("invalid coupon code")
	// ErrCouponExpired is returned when a coupon is outside its valid time window.
	ErrCouponExpired = errors.New("coupon expired")
	// ErrCouponUsageLimitReached is returned when a coupon has exhausted its allowed uses.
	ErrCouponUsageLimitReached = errors.New("coupon usage limit reached")
)

// Rule defines a coupon's discount behaviour and eligibility constraints.
type Rule struct {
	Code         string
	DiscountType DiscountType
	Value        decimal.Decimal
	MinItems     int
	Description  string
	ValidFrom    *time.Time
	ValidUntil   *time.Time
	MaxUses      int
	Uses         int
	MaxDiscount  decimal.Decimal
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

// Repository provides lookup and mutation of coupon rules.
type Repository interface {
	FindByCode(ctx context.Context, code string) (*Rule, error)
	IncrementUses(ctx context.Context, code string) error
}
