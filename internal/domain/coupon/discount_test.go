package coupon

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(v string) decimal.Decimal {
	return decimal.RequireFromString(v)
}

func TestApply(t *testing.T) {
	tests := []struct {
		name        string
		rule        *Rule
		items       []Item
		wantAmount  decimal.Decimal
		wantDesc    string
		wantErr     error
		wantErrText string
	}{
		{
			name: "percentage 18% off $100 subtotal",
			rule: &Rule{
				Code:         "PCT18",
				DiscountType: DiscountPercentage,
				Value:        d("18"),
				Description:  "18% off",
			},
			items: []Item{
				{ProductID: "p1", Price: d("50"), Quantity: 2},
			},
			wantAmount: d("18"),
			wantDesc:   "18% off",
		},
		{
			name: "percentage 50% off",
			rule: &Rule{
				Code:         "HALF",
				DiscountType: DiscountPercentage,
				Value:        d("50"),
				Description:  "50% off",
			},
			items: []Item{
				{ProductID: "p1", Price: d("80"), Quantity: 1},
			},
			wantAmount: d("40"),
			wantDesc:   "50% off",
		},
		{
			name: "percentage 100% off equals subtotal",
			rule: &Rule{
				Code:         "FREE",
				DiscountType: DiscountPercentage,
				Value:        d("100"),
				Description:  "100% off",
			},
			items: []Item{
				{ProductID: "p1", Price: d("25"), Quantity: 4},
			},
			wantAmount: d("100"),
			wantDesc:   "100% off",
		},
		{
			name: "fixed $9 off $100 subtotal",
			rule: &Rule{
				Code:         "FLAT9",
				DiscountType: DiscountFixed,
				Value:        d("9"),
				Description:  "$9 off",
			},
			items: []Item{
				{ProductID: "p1", Price: d("100"), Quantity: 1},
			},
			wantAmount: d("9"),
			wantDesc:   "$9 off",
		},
		{
			name: "fixed $200 off capped at $100 subtotal",
			rule: &Rule{
				Code:         "BIG",
				DiscountType: DiscountFixed,
				Value:        d("200"),
				Description:  "$200 off",
			},
			items: []Item{
				{ProductID: "p1", Price: d("50"), Quantity: 2},
			},
			wantAmount: d("100"),
			wantDesc:   "$200 off",
		},
		{
			name: "free lowest with 3 items",
			rule: &Rule{
				Code:         "FREELOW",
				DiscountType: DiscountFreeLowest,
				Value:        decimal.Zero,
				Description:  "free lowest item",
			},
			items: []Item{
				{ProductID: "p1", Price: d("5"), Quantity: 1},
				{ProductID: "p2", Price: d("10"), Quantity: 1},
				{ProductID: "p3", Price: d("15"), Quantity: 1},
			},
			wantAmount: d("5"),
			wantDesc:   "free lowest item",
		},
		{
			name: "free lowest with single item",
			rule: &Rule{
				Code:         "FREELOW",
				DiscountType: DiscountFreeLowest,
				Value:        decimal.Zero,
				Description:  "free lowest item",
			},
			items: []Item{
				{ProductID: "p1", Price: d("42.50"), Quantity: 1},
			},
			wantAmount: d("42.50"),
			wantDesc:   "free lowest item",
		},
		{
			name: "min items not met returns ErrInvalidCoupon",
			rule: &Rule{
				Code:         "MIN2",
				DiscountType: DiscountPercentage,
				Value:        d("10"),
				MinItems:     2,
				Description:  "10% off min 2",
			},
			items: []Item{
				{ProductID: "p1", Price: d("50"), Quantity: 1},
			},
			wantErr: ErrInvalidCoupon,
		},
		{
			name: "min items met succeeds",
			rule: &Rule{
				Code:         "MIN2",
				DiscountType: DiscountPercentage,
				Value:        d("10"),
				MinItems:     2,
				Description:  "10% off min 2",
			},
			items: []Item{
				{ProductID: "p1", Price: d("50"), Quantity: 2},
			},
			wantAmount: d("10"),
			wantDesc:   "10% off min 2",
		},
		{
			name: "empty items with zero min items",
			rule: &Rule{
				Code:         "ANY",
				DiscountType: DiscountPercentage,
				Value:        d("10"),
				MinItems:     0,
				Description:  "10% off",
			},
			items:      []Item{},
			wantAmount: d("0"),
			wantDesc:   "10% off",
		},
		{
			name: "empty items with min items required returns ErrInvalidCoupon",
			rule: &Rule{
				Code:         "MIN1",
				DiscountType: DiscountFixed,
				Value:        d("5"),
				MinItems:     1,
				Description:  "$5 off",
			},
			items:   []Item{},
			wantErr: ErrInvalidCoupon,
		},
		{
			name: "decimal precision rounds to 2 dp",
			rule: &Rule{
				Code:         "PCT33",
				DiscountType: DiscountPercentage,
				Value:        d("33.33"),
				Description:  "33.33% off",
			},
			items: []Item{
				{ProductID: "p1", Price: d("10.01"), Quantity: 1},
			},
			// 10.01 * 33.33 / 100 = 3.336333 -> rounds to 3.34
			wantAmount: d("3.34"),
			wantDesc:   "33.33% off",
		},
		{
			name: "percentage with cents precision",
			rule: &Rule{
				Code:         "PCT15",
				DiscountType: DiscountPercentage,
				Value:        d("15"),
				Description:  "15% off",
			},
			items: []Item{
				{ProductID: "p1", Price: d("9.99"), Quantity: 3},
			},
			// subtotal = 29.97, 15% = 4.4955 -> rounds to 4.50
			wantAmount: d("4.50"),
			wantDesc:   "15% off",
		},
		{
			name: "unsupported discount type returns error",
			rule: &Rule{
				Code:         "BAD",
				DiscountType: DiscountType("bogus"),
				Value:        d("10"),
				Description:  "bad type",
			},
			items: []Item{
				{ProductID: "p1", Price: d("10"), Quantity: 1},
			},
			wantErrText: "unsupported discount type",
		},
		{
			name: "free lowest min items not met returns ErrInvalidCoupon",
			rule: &Rule{
				Code:         "FL3",
				DiscountType: DiscountFreeLowest,
				Value:        decimal.Zero,
				MinItems:     3,
				Description:  "free lowest, min 3",
			},
			items: []Item{
				{ProductID: "p1", Price: d("10"), Quantity: 1},
			},
			wantErr: ErrInvalidCoupon,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Apply(tt.rule, tt.items)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			if tt.wantErrText != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrText)
				return
			}

			require.NoError(t, err)
			assert.True(t, tt.wantAmount.Equal(got.Amount),
				"expected amount %s, got %s", tt.wantAmount, got.Amount)
			assert.Equal(t, tt.wantDesc, got.Description)
		})
	}
}
