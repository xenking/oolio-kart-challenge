package api

import (
	"context"
	"fmt"

	"github.com/go-faster/errors"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/xenking/oolio-kart-challenge/internal/coupon"
	"github.com/xenking/oolio-kart-challenge/internal/oas"
	"github.com/xenking/oolio-kart-challenge/internal/order"
	"github.com/xenking/oolio-kart-challenge/internal/product"
)

// PlaceOrder validates the incoming order request, calculates pricing with
// optional coupon discounts, persists the order, and returns the full response.
func (h *Handler) PlaceOrder(ctx context.Context, req *oas.OrderReq) (oas.PlaceOrderRes, error) {
	if len(req.Items) == 0 {
		return &oas.PlaceOrderBadRequest{
			Code:    400,
			Message: "items required",
		}, nil
	}

	// Validate quantities and fetch products in one pass.
	products := make([]product.Product, 0, len(req.Items))
	for _, item := range req.Items {
		if item.Quantity <= 0 {
			return &oas.PlaceOrderUnprocessableEntity{
				Code:    422,
				Message: fmt.Sprintf("quantity must be greater than 0 for product %s", item.ProductId),
			}, nil
		}

		p, err := h.products.GetByID(ctx, item.ProductId)
		if err != nil {
			if errors.Is(err, product.ErrNotFound) {
				return &oas.PlaceOrderUnprocessableEntity{
					Code:    422,
					Message: fmt.Sprintf("product %s not found", item.ProductId),
				}, nil
			}
			return nil, errors.Wrapf(err, "get product %s", item.ProductId)
		}
		products = append(products, *p)
	}

	// Build coupon items and calculate subtotal using decimal arithmetic.
	couponItems := make([]coupon.Item, len(req.Items))
	subtotal := decimal.Zero
	for i, item := range req.Items {
		price := products[i].Price
		qty := decimal.NewFromInt(int64(item.Quantity))

		couponItems[i] = coupon.Item{
			ProductID: item.ProductId,
			Price:     price,
			Quantity:  item.Quantity,
		}
		subtotal = subtotal.Add(price.Mul(qty))
	}

	// Apply coupon discount when a code is provided.
	discountAmount := decimal.Zero
	if code, ok := req.CouponCode.Get(); ok && code != "" {
		discount, err := h.coupons.Validate(ctx, code, couponItems)
		if err != nil {
			if errors.Is(err, coupon.ErrInvalidCoupon) {
				return &oas.PlaceOrderUnprocessableEntity{
					Code:    422,
					Message: "invalid coupon code",
				}, nil
			}
			return nil, errors.Wrap(err, "validate coupon")
		}
		discountAmount = discount.Amount
	}

	// Total = subtotal - discount, floored at zero and rounded to 2 decimal places.
	total := subtotal.Sub(discountAmount)
	if total.IsNegative() {
		total = decimal.Zero
	}
	total = total.Round(2)
	discountAmount = discountAmount.Round(2)

	// Persist order.
	orderID := uuid.New().String()
	orderItems := make([]order.OrderItem, len(req.Items))
	for i, item := range req.Items {
		orderItems[i] = order.OrderItem{
			ProductID: item.ProductId,
			Quantity:  item.Quantity,
		}
	}

	couponCode := ""
	if code, ok := req.CouponCode.Get(); ok {
		couponCode = code
	}

	o := &order.Order{
		ID:         orderID,
		Items:      orderItems,
		Total:      total,
		Discounts:  discountAmount,
		CouponCode: couponCode,
	}
	if err := h.orders.Create(ctx, o); err != nil {
		return nil, errors.Wrap(err, "create order")
	}

	// Build response.
	respItems := make([]oas.OrderItem, len(req.Items))
	for i, item := range req.Items {
		respItems[i] = oas.OrderItem{
			ProductId: oas.NewOptString(item.ProductId),
			Quantity:  oas.NewOptInt(item.Quantity),
		}
	}

	respProducts := make([]oas.Product, len(products))
	for i, p := range products {
		respProducts[i] = h.domainToOASProduct(p)
	}

	return &oas.Order{
		ID:        oas.NewOptString(orderID),
		Total:     oas.NewOptFloat64(total.InexactFloat64()),
		Discounts: oas.NewOptFloat64(discountAmount.InexactFloat64()),
		Items:     respItems,
		Products:  respProducts,
	}, nil
}
