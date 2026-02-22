package handler

import (
	"github.com/xenking/oolio-kart-challenge/gen/oas"
	"github.com/xenking/oolio-kart-challenge/internal/domain/order"
	"github.com/xenking/oolio-kart-challenge/internal/domain/product"
)

// Compile-time check ensuring Handler satisfies the ogen Handler interface.
var _ oas.Handler = (*Handler)(nil)

// HandlerConfig holds non-dependency configuration for the Handler.
type HandlerConfig struct {
	// ImageBaseURL is prepended to relative image paths in product responses.
	// When empty, image paths are returned as stored in the database.
	ImageBaseURL string
}

// Handler implements the ogen-generated Handler interface, delegating business
// logic to the order service and product repository.
type Handler struct {
	oas.UnimplementedHandler

	products     product.Repository
	orderService *order.Service
	imageBaseURL string
}

// NewHandler constructs a Handler with the required domain dependencies.
func NewHandler(
	cfg HandlerConfig,
	products product.Repository,
	orderService *order.Service,
) *Handler {
	return &Handler{
		products:     products,
		orderService: orderService,
		imageBaseURL: cfg.ImageBaseURL,
	}
}
