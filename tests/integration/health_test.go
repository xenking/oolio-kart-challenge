//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestLivez(t *testing.T) {
	resp := doGet(t, "/livez")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := decodeJSON[healthResponse](t, resp)
	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}
}

func TestReadyz(t *testing.T) {
	resp := doGet(t, "/readyz")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body := decodeJSON[healthResponse](t, resp)
	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}
}
