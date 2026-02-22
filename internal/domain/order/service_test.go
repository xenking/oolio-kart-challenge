package order

import (
	"context"
	"testing"

	"github.com/go-faster/errors"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xenking/oolio-kart-challenge/internal/domain/coupon"
	"github.com/xenking/oolio-kart-challenge/internal/domain/product"
)

// --- Mock implementations ---

type mockProductRepo struct {
	byID   map[string]*product.Product
	getErr error
}

func (m *mockProductRepo) List(_ context.Context) ([]product.Product, error) {
	return nil, nil
}

func (m *mockProductRepo) GetByID(_ context.Context, id string) (*product.Product, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	p, ok := m.byID[id]
	if !ok {
		return nil, product.ErrNotFound
	}
	return p, nil
}

type mockCouponValidator struct {
	discount *coupon.Discount
	err      error
}

func (m *mockCouponValidator) Validate(_ context.Context, _ string, _ []coupon.Item) (*coupon.Discount, error) {
	return m.discount, m.err
}

type mockOrderRepo struct {
	lastOrder *Order
	err       error
}

func (m *mockOrderRepo) Create(_ context.Context, o *Order) error {
	m.lastOrder = o
	return m.err
}

// --- Helpers ---

func newTestProduct(id, name string, price decimal.Decimal) product.Product {
	return product.Product{
		ID:       id,
		Name:     name,
		Price:    price,
		Category: "test",
		Image: product.Image{
			Thumbnail: "thumb.jpg",
			Mobile:    "mobile.jpg",
			Tablet:    "tablet.jpg",
			Desktop:   "desktop.jpg",
		},
	}
}

func newProductRepo(products ...product.Product) *mockProductRepo {
	byID := make(map[string]*product.Product, len(products))
	for i := range products {
		byID[products[i].ID] = &products[i]
	}
	return &mockProductRepo{byID: byID}
}

// --- Tests ---

func TestPlaceOrder_EmptyItems(t *testing.T) {
	svc := NewService(newProductRepo(), &mockCouponValidator{}, &mockOrderRepo{})

	_, err := svc.PlaceOrder(context.Background(), PlaceOrderRequest{})
	require.ErrorIs(t, err, ErrEmptyItems)
}

func TestPlaceOrder_InvalidQuantity(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.NewFromInt(10))
	svc := NewService(newProductRepo(p1), &mockCouponValidator{}, &mockOrderRepo{})

	_, err := svc.PlaceOrder(context.Background(), PlaceOrderRequest{
		Items: []OrderItem{{ProductID: "p1", Quantity: 0}},
	})

	var iqErr *InvalidQuantityError
	require.ErrorAs(t, err, &iqErr)
	assert.Equal(t, "p1", iqErr.ProductID)
}

func TestPlaceOrder_ProductNotFound(t *testing.T) {
	svc := NewService(newProductRepo(), &mockCouponValidator{}, &mockOrderRepo{})

	_, err := svc.PlaceOrder(context.Background(), PlaceOrderRequest{
		Items: []OrderItem{{ProductID: "missing", Quantity: 1}},
	})

	var pnfErr *ProductNotFoundError
	require.ErrorAs(t, err, &pnfErr)
	assert.Equal(t, "missing", pnfErr.ProductID)
}

func TestPlaceOrder_NoCoupon(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.RequireFromString("10.00"))
	p2 := newTestProduct("p2", "Gadget", decimal.RequireFromString("20.00"))
	svc := NewService(newProductRepo(p1, p2), &mockCouponValidator{}, &mockOrderRepo{})

	result, err := svc.PlaceOrder(context.Background(), PlaceOrderRequest{
		Items: []OrderItem{
			{ProductID: "p1", Quantity: 2},
			{ProductID: "p2", Quantity: 1},
		},
	})

	require.NoError(t, err)
	assert.True(t, decimal.RequireFromString("40.00").Equal(result.Order.Total))
	assert.True(t, decimal.Zero.Equal(result.Order.Discounts))
	assert.Len(t, result.Products, 2)
}

func TestPlaceOrder_WithCoupon(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.RequireFromString("10.00"))
	p2 := newTestProduct("p2", "Gadget", decimal.RequireFromString("20.00"))
	cv := &mockCouponValidator{
		discount: &coupon.Discount{
			Amount:      decimal.RequireFromString("5.00"),
			Description: "$5 off",
		},
	}
	svc := NewService(newProductRepo(p1, p2), cv, &mockOrderRepo{})

	result, err := svc.PlaceOrder(context.Background(), PlaceOrderRequest{
		Items: []OrderItem{
			{ProductID: "p1", Quantity: 2},
			{ProductID: "p2", Quantity: 1},
		},
		CouponCode: "SAVE5",
	})

	require.NoError(t, err)
	assert.True(t, decimal.RequireFromString("35.00").Equal(result.Order.Total))
	assert.True(t, decimal.RequireFromString("5.00").Equal(result.Order.Discounts))
}

func TestPlaceOrder_InvalidCoupon(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.RequireFromString("10.00"))
	cv := &mockCouponValidator{err: coupon.ErrInvalidCoupon}
	svc := NewService(newProductRepo(p1), cv, &mockOrderRepo{})

	_, err := svc.PlaceOrder(context.Background(), PlaceOrderRequest{
		Items:      []OrderItem{{ProductID: "p1", Quantity: 1}},
		CouponCode: "BOGUS",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, coupon.ErrInvalidCoupon)
}

func TestPlaceOrder_DiscountFlooredAtZero(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.RequireFromString("10.00"))
	cv := &mockCouponValidator{
		discount: &coupon.Discount{
			Amount:      decimal.RequireFromString("999.00"),
			Description: "huge discount",
		},
	}
	svc := NewService(newProductRepo(p1), cv, &mockOrderRepo{})

	result, err := svc.PlaceOrder(context.Background(), PlaceOrderRequest{
		Items:      []OrderItem{{ProductID: "p1", Quantity: 1}},
		CouponCode: "HUGE",
	})

	require.NoError(t, err)
	assert.True(t, decimal.Zero.Equal(result.Order.Total))
	assert.True(t, decimal.RequireFromString("999.00").Equal(result.Order.Discounts))
}

func TestPlaceOrder_OrderCreateError(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.NewFromInt(10))
	svc := NewService(
		newProductRepo(p1),
		&mockCouponValidator{},
		&mockOrderRepo{err: errors.New("db write failed")},
	)

	_, err := svc.PlaceOrder(context.Background(), PlaceOrderRequest{
		Items: []OrderItem{{ProductID: "p1", Quantity: 1}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create order")
}
