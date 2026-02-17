package coupon

import (
	"github.com/go-faster/errors"
	"github.com/shopspring/decimal"
)

var (
	hundred = decimal.NewFromInt(100)
	zero    = decimal.Zero
)

// Apply calculates the discount for the given rule and cart items.
// It returns ErrInvalidCoupon when the cart does not satisfy the rule's
// minimum item count requirement.
func Apply(rule *Rule, items []Item) (Discount, error) {
	totalQty := totalQuantity(items)
	if rule.MinItems > 0 && totalQty < rule.MinItems {
		return Discount{}, ErrInvalidCoupon
	}

	subtotal := calcSubtotal(items)

	switch rule.DiscountType {
	case DiscountPercentage:
		return applyPercentage(rule, subtotal), nil
	case DiscountFixed:
		return applyFixed(rule, subtotal), nil
	case DiscountFreeLowest:
		return applyFreeLowest(rule, items), nil
	default:
		return Discount{}, errors.Errorf("unsupported discount type: %q", rule.DiscountType)
	}
}

func applyPercentage(rule *Rule, subtotal decimal.Decimal) Discount {
	amount := subtotal.Mul(rule.Value).Div(hundred)
	amount = floorAtZero(amount).Round(2)

	return Discount{
		Amount:      amount,
		Description: rule.Description,
	}
}

func applyFixed(rule *Rule, subtotal decimal.Decimal) Discount {
	amount := decimal.Min(rule.Value, subtotal)
	amount = floorAtZero(amount).Round(2)

	return Discount{
		Amount:      amount,
		Description: rule.Description,
	}
}

func applyFreeLowest(rule *Rule, items []Item) Discount {
	lowest := findLowestUnitPrice(items)

	return Discount{
		Amount:      floorAtZero(lowest).Round(2),
		Description: rule.Description,
	}
}

// calcSubtotal returns the sum of price * quantity across all items.
func calcSubtotal(items []Item) decimal.Decimal {
	sum := zero
	for _, item := range items {
		line := item.Price.Mul(decimal.NewFromInt(int64(item.Quantity)))
		sum = sum.Add(line)
	}
	return sum
}

// totalQuantity returns the sum of quantities across all items.
func totalQuantity(items []Item) int {
	total := 0
	for _, item := range items {
		total += item.Quantity
	}
	return total
}

// findLowestUnitPrice returns the lowest unit price among the given items.
// If items is empty it returns zero.
func findLowestUnitPrice(items []Item) decimal.Decimal {
	if len(items) == 0 {
		return zero
	}
	lowest := items[0].Price
	for _, item := range items[1:] {
		if item.Price.LessThan(lowest) {
			lowest = item.Price
		}
	}
	return lowest
}

// floorAtZero clamps negative values to zero.
func floorAtZero(d decimal.Decimal) decimal.Decimal {
	if d.IsNegative() {
		return zero
	}
	return d
}
