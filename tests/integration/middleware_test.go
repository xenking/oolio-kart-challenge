//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
)

func TestRequestID_Generated(t *testing.T) {
	resp := doGet(t, "/livez")
	defer resp.Body.Close()

	requestID := resp.Header.Get("X-Request-ID")
	if requestID == "" {
		t.Fatal("X-Request-ID header not present")
	}
}

func TestRequestID_Echoed(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/livez", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("X-Request-ID", "custom-request-id-12345")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	got := resp.Header.Get("X-Request-ID")
	if got != "custom-request-id-12345" {
		t.Errorf("X-Request-ID: got %q, want %q", got, "custom-request-id-12345")
	}
}

func TestCORS_Preflight(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodOptions, baseURL+"/api/product", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao == "" {
		t.Error("Access-Control-Allow-Origin header not present")
	}
	if acam := resp.Header.Get("Access-Control-Allow-Methods"); acam == "" {
		t.Error("Access-Control-Allow-Methods header not present")
	}
}

func TestCORS_SimpleRequest(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+"/api/product", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", "http://example.com")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao == "" {
		t.Error("Access-Control-Allow-Origin header not present")
	}
}

func TestRateLimit_Headers(t *testing.T) {
	resp := doGet(t, "/api/product")
	defer resp.Body.Close()

	if limit := resp.Header.Get("X-RateLimit-Limit"); limit == "" {
		t.Error("X-RateLimit-Limit header not present")
	}
	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining == "" {
		t.Error("X-RateLimit-Remaining header not present")
	}
}
