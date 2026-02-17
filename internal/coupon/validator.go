package coupon

import (
	"context"

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
}

// NewRepoValidator creates a RepoValidator backed by the given Repository.
func NewRepoValidator(repo Repository) *RepoValidator {
	return &RepoValidator{repo: repo}
}

// Validate looks up the coupon rule for the given code and applies it to the
// provided cart items. It returns ErrInvalidCoupon when the code is not found
// or the cart does not meet the rule's requirements.
func (v *RepoValidator) Validate(ctx context.Context, code string, items []Item) (*Discount, error) {
	rule, err := v.repo.FindByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrInvalidCoupon) {
			return nil, ErrInvalidCoupon
		}
		return nil, errors.Wrap(err, "lookup coupon")
	}

	d, err := Apply(rule, items)
	if err != nil {
		return nil, err
	}

	return &d, nil
}
