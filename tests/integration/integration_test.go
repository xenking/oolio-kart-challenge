//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	baseURL    string
	httpClient *http.Client
)

// Response types — defined locally to keep tests truly black-box (no internal imports).

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

type productResponse struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Price    float64      `json:"price"`
	Category string       `json:"category"`
	Image    productImage `json:"image"`
}

type productImage struct {
	Thumbnail string `json:"thumbnail"`
	Mobile    string `json:"mobile"`
	Tablet    string `json:"tablet"`
	Desktop   string `json:"desktop"`
}

type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type orderRequest struct {
	Items      []orderItemRequest `json:"items"`
	CouponCode string             `json:"couponCode,omitempty"`
}

type orderItemRequest struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

type orderResponse struct {
	ID        string            `json:"id"`
	Total     float64           `json:"total"`
	Discounts float64           `json:"discounts"`
	Items     []orderItem       `json:"items"`
	Products  []productResponse `json:"products"`
}

type orderItem struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create coverage output directory for the instrumented binary.
	if err := os.MkdirAll("coverdir", 0o777); err != nil {
		log.Fatalf("create coverdir: %v", err)
	}

	dc, err := tc.NewDockerCompose("docker-compose.test.yml")
	if err != nil {
		log.Fatalf("compose init: %v", err)
	}

	// Start postgres + api, wait until the API health check passes.
	err = dc.
		WaitForService("api", wait.ForHTTP("/readyz").WithPort("8080/tcp")).
		Up(ctx, tc.Wait(true))
	if err != nil {
		log.Fatalf("compose up: %v", err)
	}

	apiContainer, err := dc.ServiceContainer(ctx, "api")
	if err != nil {
		log.Fatalf("api container: %v", err)
	}

	host, err := apiContainer.Host(ctx)
	if err != nil {
		log.Fatalf("host: %v", err)
	}

	mappedPort, err := apiContainer.MappedPort(ctx, "8080/tcp")
	if err != nil {
		log.Fatalf("mapped port: %v", err)
	}

	baseURL = fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	httpClient = &http.Client{Timeout: 10 * time.Second}
	log.Printf("API available at %s", baseURL)

	// Seed database by running seed-db inside the already-running API container
	// (the Docker image includes the seed-db binary).
	exitCode, output, err := apiContainer.Exec(ctx, []string{
		"/app/seed-db",
		"--database-url=postgres://kart:kart@postgres:5432/kart?sslmode=disable",
		"--api-key=integration-test-key",
		"--api-key-pepper=test-pepper-for-integration",
	})
	if err != nil {
		log.Fatalf("seed exec: %v", err)
	}
	if exitCode != 0 {
		out, _ := io.ReadAll(output)
		log.Fatalf("seed-db exited %d: %s", exitCode, out)
	}
	log.Printf("seed-db completed")

	if err := waitForSeededData(ctx); err != nil {
		log.Fatalf("wait for seed: %v", err)
	}

	result := m.Run()

	// Stop the API container gracefully so the coverage-instrumented binary
	// flushes coverage data to GOCOVERDIR (bind-mounted to ./coverdir).
	// The compose file sets stop_signal: SIGINT because app.Run handles
	// SIGINT (not SIGTERM) for graceful shutdown.
	stopTimeout := 30 * time.Second
	if err := apiContainer.Stop(ctx, &stopTimeout); err != nil {
		log.Printf("stop api container: %v", err)
	}

	if err := dc.Down(context.Background(), tc.RemoveOrphans(true)); err != nil {
		log.Printf("compose down: %v", err)
	}

	return result
}

// waitForSeededData polls the product list until all 9 seeded products appear.
func waitForSeededData(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr string
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for seeded data (last: %s): %w", lastErr, ctx.Err())
		case <-ticker.C:
			resp, err := httpClient.Get(baseURL + "/api/product")
			if err != nil {
				lastErr = err.Error()
				continue
			}

			var products []productResponse
			if err := json.NewDecoder(resp.Body).Decode(&products); err != nil {
				lastErr = fmt.Sprintf("decode: %v (status: %d)", err, resp.StatusCode)
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			if len(products) == 9 {
				log.Printf("seed data ready: %d products", len(products))
				return nil
			}
			lastErr = fmt.Sprintf("got %d products, want 9", len(products))
		}
	}
}

// HTTP helpers.

func doGet(t *testing.T, path string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+path, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}

	return resp
}

func doPost(t *testing.T, path string, body any) *http.Response {
	t.Helper()

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}

	return resp
}

func doPostWithAuth(t *testing.T, path string, body any, apiKey string) *http.Response {
	t.Helper()

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api_key", apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}

	return resp
}

func decodeJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()

	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return v
}
