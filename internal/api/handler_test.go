package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/go-faster/errors"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xenking/oolio-kart-challenge/internal/coupon"
	"github.com/xenking/oolio-kart-challenge/internal/oas"
	"github.com/xenking/oolio-kart-challenge/internal/order"
	"github.com/xenking/oolio-kart-challenge/internal/product"
)

// --- Mock implementations ---

type mockProductRepo struct {
	products []product.Product
	byID     map[string]*product.Product
	listErr  error
	getErr   error
}

func (m *mockProductRepo) List(_ context.Context) ([]product.Product, error) {
	return m.products, m.listErr
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
	lastOrder *order.Order
	err       error
}

func (m *mockOrderRepo) Create(_ context.Context, o *order.Order) error {
	m.lastOrder = o
	return m.err
}

type mockAPIKeyRepo struct {
	info *APIKeyInfo
	err  error
}

func (m *mockAPIKeyRepo) FindByHash(_ context.Context, _ string) (*APIKeyInfo, error) {
	return m.info, m.err
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
	return &mockProductRepo{
		products: products,
		byID:     byID,
	}
}

// --- Tests ---

func TestListProducts(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.NewFromInt(10))
	p2 := newTestProduct("p2", "Gadget", decimal.NewFromInt(20))
	repo := newProductRepo(p1, p2)

	h := NewHandler(HandlerConfig{}, repo, &mockCouponValidator{}, &mockOrderRepo{}, &mockAPIKeyRepo{})

	result, err := h.ListProducts(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, "p1", result[0].ID.Value)
	assert.Equal(t, "Widget", result[0].Name.Value)
	assert.Equal(t, "p2", result[1].ID.Value)
	assert.Equal(t, "Gadget", result[1].Name.Value)
}

func TestListProducts_Error(t *testing.T) {
	repo := &mockProductRepo{listErr: errors.New("db down")}
	h := NewHandler(HandlerConfig{}, repo, &mockCouponValidator{}, &mockOrderRepo{}, &mockAPIKeyRepo{})

	result, err := h.ListProducts(context.Background())
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestGetProduct(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		p := newTestProduct("p1", "Widget", decimal.NewFromInt(10))
		repo := newProductRepo(p)
		h := NewHandler(HandlerConfig{}, repo, &mockCouponValidator{}, &mockOrderRepo{}, &mockAPIKeyRepo{})

		result, err := h.GetProduct(context.Background(), oas.GetProductParams{ProductId: "p1"})
		require.NoError(t, err)

		prod, ok := result.(*oas.Product)
		require.True(t, ok, "expected *oas.Product, got %T", result)
		assert.Equal(t, "p1", prod.ID.Value)
		assert.Equal(t, "Widget", prod.Name.Value)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		repo := newProductRepo() // empty
		h := NewHandler(HandlerConfig{}, repo, &mockCouponValidator{}, &mockOrderRepo{}, &mockAPIKeyRepo{})

		result, err := h.GetProduct(context.Background(), oas.GetProductParams{ProductId: "missing"})
		require.NoError(t, err)

		notFound, ok := result.(*oas.GetProductNotFound)
		require.True(t, ok, "expected *oas.GetProductNotFound, got %T", result)
		assert.Equal(t, int32(404), notFound.Code)
	})
}

func TestPlaceOrder(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.RequireFromString("10.00"))
	p2 := newTestProduct("p2", "Gadget", decimal.RequireFromString("20.00"))

	tests := []struct {
		name           string
		products       *mockProductRepo
		coupons        *mockCouponValidator
		orders         *mockOrderRepo
		req            *oas.OrderReq
		wantType       string // "order", "bad_request", "unprocessable"
		wantTotal      float64
		wantDiscounts  float64
		wantBadCode    int32
		wantBadMessage string
	}{
		{
			name:     "empty items returns 400",
			products: newProductRepo(p1),
			coupons:  &mockCouponValidator{},
			orders:   &mockOrderRepo{},
			req: &oas.OrderReq{
				Items: []oas.OrderReqItemsItem{},
			},
			wantType:       "bad_request",
			wantBadCode:    400,
			wantBadMessage: "items required",
		},
		{
			name:     "invalid quantity 0 returns 422",
			products: newProductRepo(p1),
			coupons:  &mockCouponValidator{},
			orders:   &mockOrderRepo{},
			req: &oas.OrderReq{
				Items: []oas.OrderReqItemsItem{
					{ProductId: "p1", Quantity: 0},
				},
			},
			wantType:    "unprocessable",
			wantBadCode: 422,
		},
		{
			name:     "product not found returns 422",
			products: newProductRepo(p1),
			coupons:  &mockCouponValidator{},
			orders:   &mockOrderRepo{},
			req: &oas.OrderReq{
				Items: []oas.OrderReqItemsItem{
					{ProductId: "nonexistent", Quantity: 1},
				},
			},
			wantType:       "unprocessable",
			wantBadCode:    422,
			wantBadMessage: "product nonexistent not found",
		},
		{
			name:     "valid order without coupon",
			products: newProductRepo(p1, p2),
			coupons:  &mockCouponValidator{},
			orders:   &mockOrderRepo{},
			req: &oas.OrderReq{
				Items: []oas.OrderReqItemsItem{
					{ProductId: "p1", Quantity: 2},
					{ProductId: "p2", Quantity: 1},
				},
			},
			wantType:      "order",
			wantTotal:     40.00, // 10*2 + 20*1
			wantDiscounts: 0,
		},
		{
			name:     "valid order with coupon applies discount",
			products: newProductRepo(p1, p2),
			coupons: &mockCouponValidator{
				discount: &coupon.Discount{
					Amount:      decimal.RequireFromString("5.00"),
					Description: "$5 off",
				},
			},
			orders: &mockOrderRepo{},
			req: &oas.OrderReq{
				CouponCode: oas.NewOptString("SAVE5"),
				Items: []oas.OrderReqItemsItem{
					{ProductId: "p1", Quantity: 2},
					{ProductId: "p2", Quantity: 1},
				},
			},
			wantType:      "order",
			wantTotal:     35.00, // 40 - 5
			wantDiscounts: 5.00,
		},
		{
			name:     "invalid coupon returns 422",
			products: newProductRepo(p1),
			coupons: &mockCouponValidator{
				err: coupon.ErrInvalidCoupon,
			},
			orders: &mockOrderRepo{},
			req: &oas.OrderReq{
				CouponCode: oas.NewOptString("BOGUS"),
				Items: []oas.OrderReqItemsItem{
					{ProductId: "p1", Quantity: 1},
				},
			},
			wantType:       "unprocessable",
			wantBadCode:    422,
			wantBadMessage: "invalid coupon code",
		},
		{
			name:     "discount larger than subtotal floors total at 0",
			products: newProductRepo(p1),
			coupons: &mockCouponValidator{
				discount: &coupon.Discount{
					Amount:      decimal.RequireFromString("999.00"),
					Description: "huge discount",
				},
			},
			orders: &mockOrderRepo{},
			req: &oas.OrderReq{
				CouponCode: oas.NewOptString("HUGE"),
				Items: []oas.OrderReqItemsItem{
					{ProductId: "p1", Quantity: 1}, // subtotal = 10
				},
			},
			wantType:      "order",
			wantTotal:     0,
			wantDiscounts: 999.00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(HandlerConfig{}, tt.products, tt.coupons, tt.orders, &mockAPIKeyRepo{})

			result, err := h.PlaceOrder(context.Background(), tt.req)
			require.NoError(t, err)

			switch tt.wantType {
			case "bad_request":
				resp, ok := result.(*oas.PlaceOrderBadRequest)
				require.True(t, ok, "expected *oas.PlaceOrderBadRequest, got %T", result)
				assert.Equal(t, tt.wantBadCode, resp.Code)
				if tt.wantBadMessage != "" {
					assert.Equal(t, tt.wantBadMessage, resp.Message)
				}

			case "unprocessable":
				resp, ok := result.(*oas.PlaceOrderUnprocessableEntity)
				require.True(t, ok, "expected *oas.PlaceOrderUnprocessableEntity, got %T", result)
				assert.Equal(t, tt.wantBadCode, resp.Code)
				if tt.wantBadMessage != "" {
					assert.Equal(t, tt.wantBadMessage, resp.Message)
				}

			case "order":
				resp, ok := result.(*oas.Order)
				require.True(t, ok, "expected *oas.Order, got %T", result)
				assert.True(t, resp.ID.IsSet(), "order ID should be set")
				assert.NotEmpty(t, resp.ID.Value)
				assert.InDelta(t, tt.wantTotal, resp.Total.Value, 0.01)
				assert.InDelta(t, tt.wantDiscounts, resp.Discounts.Value, 0.01)
			}
		})
	}
}

func TestPlaceOrder_OrderCreateError(t *testing.T) {
	p1 := newTestProduct("p1", "Widget", decimal.NewFromInt(10))
	h := NewHandler(
		HandlerConfig{},
		newProductRepo(p1),
		&mockCouponValidator{},
		&mockOrderRepo{err: errors.New("db write failed")},
		&mockAPIKeyRepo{},
	)

	req := &oas.OrderReq{
		Items: []oas.OrderReqItemsItem{
			{ProductId: "p1", Quantity: 1},
		},
	}

	result, err := h.PlaceOrder(context.Background(), req)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "create order")
}

func TestHandleAPIKey(t *testing.T) {
	t.Run("valid key returns context", func(t *testing.T) {
		// Compute the SHA-256 hex hash the handler will produce for the key.
		apiKey := "my-secret-key"
		hash := sha256.Sum256([]byte(apiKey))
		hexHash := hex.EncodeToString(hash[:])

		h := NewHandler(
			HandlerConfig{},
			&mockProductRepo{},
			&mockCouponValidator{},
			&mockOrderRepo{},
			&mockAPIKeyRepo{
				info: &APIKeyInfo{
					ID:      "key-1",
					KeyHash: hexHash,
					Name:    "test-key",
					Scopes:  []string{"orders:write"},
				},
			},
		)

		ctx := context.Background()
		resultCtx, err := h.HandleAPIKey(ctx, oas.PlaceOrderOperation, oas.APIKey{APIKey: apiKey})
		require.NoError(t, err)
		assert.NotNil(t, resultCtx)
	})

	t.Run("invalid key returns error", func(t *testing.T) {
		h := NewHandler(
			HandlerConfig{},
			&mockProductRepo{},
			&mockCouponValidator{},
			&mockOrderRepo{},
			&mockAPIKeyRepo{
				err: errors.New("not found"),
			},
		)

		ctx := context.Background()
		_, err := h.HandleAPIKey(ctx, oas.PlaceOrderOperation, oas.APIKey{APIKey: "bad-key"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unauthorized")
	})
}
