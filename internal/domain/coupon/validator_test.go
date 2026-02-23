package coupon

import (
	"context"
	"testing"
	"time"

	"github.com/go-faster/errors"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCouponRepo struct {
	rule          *Rule
	err           error
	incrementErr  error
	incrementCode string
}

func (m *mockCouponRepo) FindByCode(_ context.Context, _ string) (*Rule, error) {
	return m.rule, m.err
}

func (m *mockCouponRepo) IncrementUses(_ context.Context, code string) error {
	m.incrementCode = code
	return m.incrementErr
}

func TestRepoValidator_Validate(t *testing.T) {
	fixedNow := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	pastTime := fixedNow.Add(-24 * time.Hour)
	futureTime := fixedNow.Add(24 * time.Hour)
	farFuture := fixedNow.Add(48 * time.Hour)

	tests := []struct {
		name       string
		repo       *mockCouponRepo
		code       string
		items      []Item
		wantAmount decimal.Decimal
		wantErr    error
	}{
		{
			name: "valid code returns discount",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "SAVE10",
					DiscountType: DiscountPercentage,
					Value:        decimal.NewFromInt(10),
					Description:  "10% off",
				},
			},
			code: "SAVE10",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
			},
			wantAmount: decimal.NewFromInt(10),
		},
		{
			name: "unknown code returns ErrInvalidCoupon",
			repo: &mockCouponRepo{
				err: ErrInvalidCoupon,
			},
			code: "BOGUS",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(50), Quantity: 1},
			},
			wantErr: ErrInvalidCoupon,
		},
		{
			name: "code found but items do not meet MinItems returns ErrInvalidCoupon",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "MIN3",
					DiscountType: DiscountFixed,
					Value:        decimal.NewFromInt(5),
					MinItems:     3,
					Description:  "$5 off (min 3)",
				},
			},
			code: "MIN3",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(20), Quantity: 1},
			},
			wantErr: ErrInvalidCoupon,
		},
		{
			name: "code found and items meet MinItems returns discount",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "MIN2",
					DiscountType: DiscountFixed,
					Value:        decimal.NewFromInt(5),
					MinItems:     2,
					Description:  "$5 off (min 2)",
				},
			},
			code: "MIN2",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(20), Quantity: 1},
				{ProductID: "p2", Price: decimal.NewFromInt(30), Quantity: 1},
			},
			wantAmount: decimal.NewFromInt(5),
		},
		{
			name: "expired coupon (valid_until in past)",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "OLD",
					DiscountType: DiscountPercentage,
					Value:        decimal.NewFromInt(10),
					ValidUntil:   &pastTime,
					Description:  "expired",
				},
			},
			code: "OLD",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
			},
			wantErr: ErrCouponExpired,
		},
		{
			name: "coupon not yet valid (valid_from in future)",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "FUTURE",
					DiscountType: DiscountPercentage,
					Value:        decimal.NewFromInt(10),
					ValidFrom:    &futureTime,
					Description:  "not yet",
				},
			},
			code: "FUTURE",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
			},
			wantErr: ErrCouponExpired,
		},
		{
			name: "coupon within valid window succeeds",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "WINDOW",
					DiscountType: DiscountPercentage,
					Value:        decimal.NewFromInt(10),
					ValidFrom:    &pastTime,
					ValidUntil:   &futureTime,
					Description:  "valid window",
				},
			},
			code: "WINDOW",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
			},
			wantAmount: decimal.NewFromInt(10),
		},
		{
			name: "coupon with valid_from=nil and valid_until in future succeeds",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "NOSTART",
					DiscountType: DiscountFixed,
					Value:        decimal.NewFromInt(5),
					ValidUntil:   &farFuture,
					Description:  "no start",
				},
			},
			code: "NOSTART",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
			},
			wantAmount: decimal.NewFromInt(5),
		},
		{
			name: "usage limit reached",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "LIMITED",
					DiscountType: DiscountPercentage,
					Value:        decimal.NewFromInt(10),
					MaxUses:      100,
					Uses:         100,
					Description:  "limited",
				},
			},
			code: "LIMITED",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
			},
			wantErr: ErrCouponUsageLimitReached,
		},
		{
			name: "usage under limit succeeds",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "HASROOM",
					DiscountType: DiscountPercentage,
					Value:        decimal.NewFromInt(10),
					MaxUses:      100,
					Uses:         50,
					Description:  "has room",
				},
			},
			code: "HASROOM",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
			},
			wantAmount: decimal.NewFromInt(10),
		},
		{
			name: "unlimited uses (max_uses=0) always succeeds",
			repo: &mockCouponRepo{
				rule: &Rule{
					Code:         "UNLIMITED",
					DiscountType: DiscountFixed,
					Value:        decimal.NewFromInt(5),
					MaxUses:      0,
					Uses:         9999,
					Description:  "unlimited",
				},
			},
			code: "UNLIMITED",
			items: []Item{
				{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
			},
			wantAmount: decimal.NewFromInt(5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewRepoValidator(tt.repo)
			v.now = func() time.Time { return fixedNow }

			got, err := v.Validate(context.Background(), tt.code, tt.items)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.True(t, tt.wantAmount.Equal(got.Amount),
				"expected amount %s, got %s", tt.wantAmount, got.Amount)
		})
	}
}

func TestRepoValidator_IncrementUsesCalledOnSuccess(t *testing.T) {
	repo := &mockCouponRepo{
		rule: &Rule{
			Code:         "INC",
			DiscountType: DiscountFixed,
			Value:        decimal.NewFromInt(5),
			Description:  "inc test",
		},
	}

	v := NewRepoValidator(repo)
	_, err := v.Validate(context.Background(), "INC", []Item{
		{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
	})

	require.NoError(t, err)
	assert.Equal(t, "INC", repo.incrementCode)
}

func TestRepoValidator_IncrementUsesError(t *testing.T) {
	repo := &mockCouponRepo{
		rule: &Rule{
			Code:         "FAIL",
			DiscountType: DiscountFixed,
			Value:        decimal.NewFromInt(5),
			Description:  "fail test",
		},
		incrementErr: errors.New("db error"),
	}

	v := NewRepoValidator(repo)
	_, err := v.Validate(context.Background(), "FAIL", []Item{
		{ProductID: "p1", Price: decimal.NewFromInt(100), Quantity: 1},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "increment coupon uses")
}
