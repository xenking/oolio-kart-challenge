//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestListProducts(t *testing.T) {
	resp := doGet(t, "/api/product")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	products := decodeJSON[[]productResponse](t, resp)
	if len(products) != 9 {
		t.Fatalf("expected 9 products, got %d", len(products))
	}
}

func TestListProducts_Fields(t *testing.T) {
	resp := doGet(t, "/api/product")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	products := decodeJSON[[]productResponse](t, resp)

	var waffle *productResponse
	for i := range products {
		if products[i].ID == "1" {
			waffle = &products[i]
			break
		}
	}

	if waffle == nil {
		t.Fatal("product with ID '1' not found")
	}
	if waffle.Name != "Waffle with Berries" {
		t.Errorf("name: got %q, want %q", waffle.Name, "Waffle with Berries")
	}
	if waffle.Price != 6.5 {
		t.Errorf("price: got %v, want 6.5", waffle.Price)
	}
	if waffle.Category != "Waffle" {
		t.Errorf("category: got %q, want %q", waffle.Category, "Waffle")
	}
	if waffle.Image.Thumbnail == "" {
		t.Error("image.thumbnail is empty")
	}
	if waffle.Image.Mobile == "" {
		t.Error("image.mobile is empty")
	}
	if waffle.Image.Tablet == "" {
		t.Error("image.tablet is empty")
	}
	if waffle.Image.Desktop == "" {
		t.Error("image.desktop is empty")
	}
}

func TestGetProduct(t *testing.T) {
	resp := doGet(t, "/product/1")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	product := decodeJSON[productResponse](t, resp)
	if product.ID != "1" {
		t.Errorf("id: got %q, want %q", product.ID, "1")
	}
	if product.Name != "Waffle with Berries" {
		t.Errorf("name: got %q, want %q", product.Name, "Waffle with Berries")
	}
}

func TestGetProduct_NotFound(t *testing.T) {
	resp := doGet(t, "/product/999")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	errResp := decodeJSON[errorResponse](t, resp)
	if errResp.Code != 404 {
		t.Errorf("error code: got %d, want 404", errResp.Code)
	}
}
