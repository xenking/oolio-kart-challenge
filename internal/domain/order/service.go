package order

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/xenking/oolio-kart-challenge/internal/domain/coupon"
	"github.com/xenking/oolio-kart-challenge/internal/domain/product"
)

// Sentinel errors for order validation.
var (
	ErrEmptyItems      = fmt.Errorf("items required")
	ErrInvalidQuantity = fmt.Errorf("quantity must be greater than 0")
)

// ProductNotFoundError indicates a requested product does not exist.
type ProductNotFoundError struct {
	ProductID string
}

func (e *ProductNotFoundError) Error() string {
	return fmt.Sprintf("product %s not found", e.ProductID)
}

// InvalidQuantityError indicates a line item has a non-positive quantity.
type InvalidQuantityError struct {
	ProductID string
}

func (e *InvalidQuantityError) Error() string {
	return fmt.Sprintf("quantity must be greater than 0 for product %s", e.ProductID)
}

// PlaceOrderRequest holds the input for placing an order.
type PlaceOrderRequest struct {
	Items      []OrderItem
	CouponCode string
}

// PlaceOrderResult holds the output of a successfully placed order.
type PlaceOrderResult struct {
	Order    *Order
	Products []product.Product
}

// Service encapsulates order placement business logic.
type Service struct {
	products product.Repository
	coupons  coupon.Validator
	orders   Repository
}

// NewService creates an order Service with the required domain dependencies.
func NewService(
	products product.Repository,
	coupons coupon.Validator,
	orders Repository,
) *Service {
	return &Service{
		products: products,
		coupons:  coupons,
		orders:   orders,
	}
}

// PlaceOrder validates items, fetches products in a single batch, applies
// coupons, persists the order, and returns the result.
func (s *Service) PlaceOrder(ctx context.Context, req PlaceOrderRequest) (*PlaceOrderResult, error) {
	if len(req.Items) == 0 {
		return nil, ErrEmptyItems
	}

	// Validate quantities and collect product IDs.
	ids := make([]string, len(req.Items))
	for i, item := range req.Items {
		if item.Quantity <= 0 {
			return nil, &InvalidQuantityError{ProductID: item.ProductID}
		}
		ids[i] = item.ProductID
	}

	// Batch fetch all products in a single query.
	fetched, err := s.products.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get products: %w", err)
	}

	productMap := make(map[string]product.Product, len(fetched))
	for _, p := range fetched {
		productMap[p.ID] = p
	}

	// Verify every requested product was found.
	products := make([]product.Product, 0, len(req.Items))
	for _, item := range req.Items {
		p, ok := productMap[item.ProductID]
		if !ok {
			return nil, &ProductNotFoundError{ProductID: item.ProductID}
		}
		products = append(products, p)
	}

	// Build coupon items and calculate subtotal.
	couponItems := make([]coupon.Item, len(req.Items))
	subtotal := decimal.Zero
	for i, item := range req.Items {
		price := products[i].Price
		qty := decimal.NewFromInt(int64(item.Quantity))

		couponItems[i] = coupon.Item{
			ProductID: item.ProductID,
			Price:     price,
			Quantity:  item.Quantity,
		}
		subtotal = subtotal.Add(price.Mul(qty))
	}

	// Apply coupon discount when a code is provided.
	discountAmount := decimal.Zero
	if req.CouponCode != "" {
		discount, err := s.coupons.Validate(ctx, req.CouponCode, couponItems)
		if err != nil {
			return nil, fmt.Errorf("validate coupon: %w", err)
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
	o := &Order{
		ID:         uuid.New().String(),
		Items:      req.Items,
		Total:      total,
		Discounts:  discountAmount,
		CouponCode: req.CouponCode,
	}
	if err := s.orders.Create(ctx, o); err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}

	return &PlaceOrderResult{
		Order:    o,
		Products: products,
	}, nil
}
