package coupon

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCouponRepo struct {
	rule *Rule
	err  error
}

func (m *mockCouponRepo) FindByCode(_ context.Context, _ string) (*Rule, error) {
	return m.rule, m.err
}

func TestRepoValidator_Validate(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewRepoValidator(tt.repo)
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
