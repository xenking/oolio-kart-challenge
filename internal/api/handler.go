package api

import (
	"context"

	"github.com/xenking/oolio-kart-challenge/internal/coupon"
	"github.com/xenking/oolio-kart-challenge/internal/oas"
	"github.com/xenking/oolio-kart-challenge/internal/order"
	"github.com/xenking/oolio-kart-challenge/internal/product"
)

// Compile-time checks ensuring Handler satisfies both ogen interfaces.
var (
	_ oas.Handler         = (*Handler)(nil)
	_ oas.SecurityHandler = (*Handler)(nil)
)

// APIKeyInfo holds the identity and permission data for a validated API key.
type APIKeyInfo struct {
	ID      string
	KeyHash string
	Name    string
	Scopes  []string
}

// APIKeyRepository provides lookup of API keys by their SHA-256 hash.
type APIKeyRepository interface {
	FindByHash(ctx context.Context, hash string) (*APIKeyInfo, error)
}

// HandlerConfig holds non-dependency configuration for the Handler.
type HandlerConfig struct {
	// ImageBaseURL is prepended to relative image paths in product responses.
	// When empty, image paths are returned as stored in the database.
	ImageBaseURL string
}

// Handler implements the ogen-generated Handler and SecurityHandler interfaces,
// delegating business logic to the injected domain repositories and validators.
type Handler struct {
	oas.UnimplementedHandler

	products     product.Repository
	coupons      coupon.Validator
	orders       order.Repository
	apikeys      APIKeyRepository
	imageBaseURL string
}

// NewHandler constructs a Handler with the required domain dependencies.
func NewHandler(
	cfg HandlerConfig,
	products product.Repository,
	coupons coupon.Validator,
	orders order.Repository,
	apikeys APIKeyRepository,
) *Handler {
	return &Handler{
		products:     products,
		coupons:      coupons,
		orders:       orders,
		apikeys:      apikeys,
		imageBaseURL: cfg.ImageBaseURL,
	}
}
