package handler

import (
	"context"

	"github.com/go-faster/errors"

	"github.com/xenking/oolio-kart-challenge/gen/oas"
	"github.com/xenking/oolio-kart-challenge/internal/domain/coupon"
	"github.com/xenking/oolio-kart-challenge/internal/domain/order"
)

// PlaceOrder converts the OAS request to a domain request, delegates to the
// order service, and maps the result (or error) back to an OAS response.
func (h *Handler) PlaceOrder(ctx context.Context, req *oas.OrderReq) (oas.PlaceOrderRes, error) {
	// Convert OAS types to domain request.
	items := make([]order.OrderItem, len(req.Items))
	for i, item := range req.Items {
		items[i] = order.OrderItem{
			ProductID: item.ProductId,
			Quantity:  item.Quantity,
		}
	}

	couponCode := ""
	if code, ok := req.CouponCode.Get(); ok {
		couponCode = code
	}

	result, err := h.orderService.PlaceOrder(ctx, order.PlaceOrderRequest{
		Items:      items,
		CouponCode: couponCode,
	})
	if err != nil {
		return mapOrderError(err)
	}

	// Build OAS response.
	respItems := make([]oas.OrderItem, len(req.Items))
	for i, item := range req.Items {
		respItems[i] = oas.OrderItem{
			ProductId: oas.NewOptString(item.ProductId),
			Quantity:  oas.NewOptInt(item.Quantity),
		}
	}

	respProducts := make([]oas.Product, len(result.Products))
	for i, p := range result.Products {
		respProducts[i] = h.domainToOASProduct(p)
	}

	return &oas.Order{
		ID:        oas.NewOptString(result.Order.ID),
		Total:     oas.NewOptFloat64(result.Order.Total.InexactFloat64()),
		Discounts: oas.NewOptFloat64(result.Order.Discounts.InexactFloat64()),
		Items:     respItems,
		Products:  respProducts,
	}, nil
}

// mapOrderError converts domain errors to OAS error responses.
func mapOrderError(err error) (oas.PlaceOrderRes, error) {
	if errors.Is(err, order.ErrEmptyItems) {
		return &oas.PlaceOrderBadRequest{
			Code:    400,
			Message: err.Error(),
		}, nil
	}

	var iqErr *order.InvalidQuantityError
	if errors.As(err, &iqErr) {
		return &oas.PlaceOrderUnprocessableEntity{
			Code:    422,
			Message: iqErr.Error(),
		}, nil
	}

	var pnfErr *order.ProductNotFoundError
	if errors.As(err, &pnfErr) {
		return &oas.PlaceOrderUnprocessableEntity{
			Code:    422,
			Message: pnfErr.Error(),
		}, nil
	}

	if errors.Is(err, coupon.ErrInvalidCoupon) {
		return &oas.PlaceOrderUnprocessableEntity{
			Code:    422,
			Message: "invalid coupon code",
		}, nil
	}

	return nil, err
}
