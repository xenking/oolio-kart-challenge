package postgres

import (
	"context"
	"fmt"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xenking/oolio-kart-challenge/internal/coupon"
	"github.com/xenking/oolio-kart-challenge/internal/dbgen"
)

var _ coupon.Repository = (*CouponRepository)(nil)

// CouponRepository implements coupon.Repository backed by PostgreSQL.
type CouponRepository struct {
	q *dbgen.Queries
}

// NewCouponRepository returns a CouponRepository that uses the given pool.
func NewCouponRepository(pool *pgxpool.Pool) *CouponRepository {
	return &CouponRepository{q: dbgen.New(pool)}
}

// FindByCode looks up an active coupon by its code. The SQL query applies
// UPPER() on the parameter, so the code is passed as-is.
// Returns coupon.ErrInvalidCoupon when no matching active coupon exists.
func (r *CouponRepository) FindByCode(ctx context.Context, code string) (*coupon.Rule, error) {
	row, err := r.q.GetCouponByCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, coupon.ErrInvalidCoupon
		}
		return nil, fmt.Errorf("finding coupon by code %q: %w", code, err)
	}

	rule := mapCouponRule(row)
	return &rule, nil
}

func mapCouponRule(row dbgen.GetCouponByCodeRow) coupon.Rule {
	return coupon.Rule{
		Code:         row.Code,
		DiscountType: coupon.DiscountType(row.DiscountType),
		Value:        row.Value,
		MinItems:     int(row.MinItems),
		Description:  row.Description,
	}
}
