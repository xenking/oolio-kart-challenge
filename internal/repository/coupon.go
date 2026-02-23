package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/xenking/oolio-kart-challenge/internal/domain/coupon"
)

const (
	getCouponByCodeSQL = `SELECT code, discount_type, value, min_items, description,
		valid_from, valid_until, max_uses, uses, max_discount
		FROM coupons WHERE UPPER(code) = UPPER($1) AND active = TRUE`

	incrementCouponUsesSQL = `UPDATE coupons SET uses = uses + 1 WHERE code = $1`
)

var _ coupon.Repository = (*CouponRepository)(nil)

// CouponRepository implements coupon.Repository backed by PostgreSQL.
type CouponRepository struct {
	pool *pgxpool.Pool
}

// NewCouponRepository returns a CouponRepository that uses the given pool.
func NewCouponRepository(pool *pgxpool.Pool) *CouponRepository {
	return &CouponRepository{pool: pool}
}

// FindByCode looks up an active coupon by its code (case-insensitive).
// Returns coupon.ErrInvalidCoupon when no matching active coupon exists.
func (r *CouponRepository) FindByCode(ctx context.Context, code string) (*coupon.Rule, error) {
	rows, err := r.pool.Query(ctx, getCouponByCodeSQL, code)
	if err != nil {
		return nil, fmt.Errorf("finding coupon by code %q: %w", code, err)
	}

	rule, err := pgx.CollectExactlyOneRow(rows, scanCouponRule)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, coupon.ErrInvalidCoupon
		}
		return nil, fmt.Errorf("finding coupon by code %q: %w", code, err)
	}
	return &rule, nil
}

// IncrementUses atomically increments the usage counter for the given coupon code.
func (r *CouponRepository) IncrementUses(ctx context.Context, code string) error {
	_, err := r.pool.Exec(ctx, incrementCouponUsesSQL, code)
	if err != nil {
		return fmt.Errorf("incrementing uses for coupon %q: %w", code, err)
	}
	return nil
}

func scanCouponRule(row pgx.CollectableRow) (coupon.Rule, error) {
	var (
		rule         coupon.Rule
		discountType string
		value        decimal.Decimal
		minItems     int32
		validFrom    *time.Time
		validUntil   *time.Time
		maxUses      int32
		uses         int32
		maxDiscount  decimal.Decimal
	)
	err := row.Scan(
		&rule.Code, &discountType, &value, &minItems, &rule.Description,
		&validFrom, &validUntil, &maxUses, &uses, &maxDiscount,
	)
	rule.DiscountType = coupon.DiscountType(discountType)
	rule.Value = value
	rule.MinItems = int(minItems)
	rule.ValidFrom = validFrom
	rule.ValidUntil = validUntil
	rule.MaxUses = int(maxUses)
	rule.Uses = int(uses)
	rule.MaxDiscount = maxDiscount
	return rule, err
}
