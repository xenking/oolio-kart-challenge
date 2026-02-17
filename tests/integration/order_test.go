//go:build integration

package integration

import (
	"net/http"
	"regexp"
	"testing"
)

const testAPIKey = "apitest"

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestPlaceOrder_NoAuth(t *testing.T) {
	req := orderRequest{
		Items: []orderItemRequest{{ProductID: "1", Quantity: 1}},
	}
	resp := doPost(t, "/api/order", req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPlaceOrder_InvalidKey(t *testing.T) {
	req := orderRequest{
		Items: []orderItemRequest{{ProductID: "1", Quantity: 1}},
	}
	resp := doPostWithAuth(t, "/api/order", req, "wrong-key")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestPlaceOrder_EmptyItems(t *testing.T) {
	req := orderRequest{
		Items: []orderItemRequest{},
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestPlaceOrder_InvalidProduct(t *testing.T) {
	req := orderRequest{
		Items: []orderItemRequest{{ProductID: "999", Quantity: 1}},
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestPlaceOrder_SingleItem(t *testing.T) {
	req := orderRequest{
		Items: []orderItemRequest{{ProductID: "1", Quantity: 1}}, // Waffle $6.50
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	order := decodeJSON[orderResponse](t, resp)
	if order.Total != 6.5 {
		t.Errorf("total: got %v, want 6.5", order.Total)
	}
	if order.Discounts != 0 {
		t.Errorf("discounts: got %v, want 0", order.Discounts)
	}
}

func TestPlaceOrder_MultipleItems(t *testing.T) {
	req := orderRequest{
		Items: []orderItemRequest{
			{ProductID: "1", Quantity: 2}, // 2x Waffle $6.50 = $13.00
			{ProductID: "2", Quantity: 1}, // 1x Creme Brulee $7.00
		},
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	order := decodeJSON[orderResponse](t, resp)
	if order.Total != 20 {
		t.Errorf("total: got %v, want 20", order.Total)
	}
}

func TestPlaceOrder_HappyHours(t *testing.T) {
	req := orderRequest{
		Items:      []orderItemRequest{{ProductID: "3", Quantity: 1}}, // Macaron $8.00
		CouponCode: "HAPPYHOURS",
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	order := decodeJSON[orderResponse](t, resp)
	// 8.00 * 18% = 1.44
	if order.Discounts != 1.44 {
		t.Errorf("discounts: got %v, want 1.44", order.Discounts)
	}
	// 8.00 - 1.44 = 6.56
	if order.Total != 6.56 {
		t.Errorf("total: got %v, want 6.56", order.Total)
	}
}

func TestPlaceOrder_BuyGetOne(t *testing.T) {
	req := orderRequest{
		Items: []orderItemRequest{
			{ProductID: "1", Quantity: 1}, // Waffle $6.50
			{ProductID: "5", Quantity: 1}, // Baklava $4.00
		},
		CouponCode: "BUYGETONE",
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	order := decodeJSON[orderResponse](t, resp)
	// Lowest price item free: Baklava $4.00
	if order.Discounts != 4 {
		t.Errorf("discounts: got %v, want 4", order.Discounts)
	}
	// 6.50 + 4.00 - 4.00 = 6.50
	if order.Total != 6.5 {
		t.Errorf("total: got %v, want 6.5", order.Total)
	}
}

func TestPlaceOrder_BuyGetOne_InsufficientItems(t *testing.T) {
	req := orderRequest{
		Items:      []orderItemRequest{{ProductID: "1", Quantity: 1}}, // Only 1 item, needs 2
		CouponCode: "BUYGETONE",
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestPlaceOrder_InvalidCoupon(t *testing.T) {
	req := orderRequest{
		Items:      []orderItemRequest{{ProductID: "1", Quantity: 1}},
		CouponCode: "NONEXISTENT",
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestPlaceOrder_ResponseStructure(t *testing.T) {
	req := orderRequest{
		Items: []orderItemRequest{{ProductID: "1", Quantity: 1}},
	}
	resp := doPostWithAuth(t, "/api/order", req, testAPIKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	order := decodeJSON[orderResponse](t, resp)

	if !uuidPattern.MatchString(order.ID) {
		t.Errorf("order ID %q is not a valid UUID", order.ID)
	}
	if len(order.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(order.Items))
	}
	if len(order.Products) != 1 {
		t.Fatalf("expected 1 product, got %d", len(order.Products))
	}

	product := order.Products[0]
	if product.ID != "1" {
		t.Errorf("product id: got %q, want %q", product.ID, "1")
	}
	if product.Name == "" {
		t.Error("product name is empty")
	}
	if product.Price <= 0 {
		t.Errorf("product price: got %v, want > 0", product.Price)
	}
}
