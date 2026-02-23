package coupon

import (
	"context"
	"time"

	"github.com/go-faster/errors"
)

// Validator validates a coupon code against a set of cart items and returns
// the computed discount.
type Validator interface {
	Validate(ctx context.Context, code string, items []Item) (*Discount, error)
}

// RepoValidator implements Validator by looking up coupon rules from a
// Repository and applying them via the Apply function.
type RepoValidator struct {
	repo Repository
	now  func() time.Time
}

// NewRepoValidator creates a RepoValidator backed by the given Repository.
func NewRepoValidator(repo Repository) *RepoValidator {
	return &RepoValidator{repo: repo, now: time.Now}
}

// Validate looks up the coupon rule for the given code, checks temporal
// validity and usage limits, applies it to the cart items, and increments
// the usage counter on success.
func (v *RepoValidator) Validate(ctx context.Context, code string, items []Item) (*Discount, error) {
	rule, err := v.repo.FindByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrInvalidCoupon) {
			return nil, ErrInvalidCoupon
		}
		return nil, errors.Wrap(err, "lookup coupon")
	}

	now := v.now()

	if rule.ValidFrom != nil && now.Before(*rule.ValidFrom) {
		return nil, ErrCouponExpired
	}
	if rule.ValidUntil != nil && now.After(*rule.ValidUntil) {
		return nil, ErrCouponExpired
	}

	if rule.MaxUses > 0 && rule.Uses >= rule.MaxUses {
		return nil, ErrCouponUsageLimitReached
	}

	d, err := Apply(rule, items)
	if err != nil {
		return nil, err
	}

	if err := v.repo.IncrementUses(ctx, code); err != nil {
		return nil, errors.Wrap(err, "increment coupon uses")
	}

	return &d, nil
}
