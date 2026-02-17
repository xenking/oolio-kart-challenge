package api

import (
	"context"

	"github.com/go-faster/errors"

	"github.com/xenking/oolio-kart-challenge/internal/oas"
	"github.com/xenking/oolio-kart-challenge/internal/product"
)

// ListProducts returns every product in the catalog.
func (h *Handler) ListProducts(ctx context.Context) ([]oas.Product, error) {
	products, err := h.products.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list products")
	}

	out := make([]oas.Product, len(products))
	for i, p := range products {
		out[i] = h.domainToOASProduct(p)
	}
	return out, nil
}

// GetProduct returns a single product by ID.
func (h *Handler) GetProduct(ctx context.Context, params oas.GetProductParams) (oas.GetProductRes, error) {
	p, err := h.products.GetByID(ctx, params.ProductId)
	if err != nil {
		if errors.Is(err, product.ErrNotFound) {
			return &oas.GetProductNotFound{
				Code:    404,
				Message: "product not found",
			}, nil
		}
		return nil, errors.Wrap(err, "get product")
	}

	result := h.domainToOASProduct(*p)
	return &result, nil
}

// domainToOASProduct converts a domain product into the ogen response type.
// Image paths are prefixed with the configured imageBaseURL.
func (h *Handler) domainToOASProduct(p product.Product) oas.Product {
	base := h.imageBaseURL
	return oas.Product{
		ID:       oas.NewOptString(p.ID),
		Name:     oas.NewOptString(p.Name),
		Price:    oas.NewOptFloat32(float32(p.Price.InexactFloat64())),
		Category: oas.NewOptString(p.Category),
		Image: oas.NewOptProductImage(oas.ProductImage{
			Thumbnail: oas.NewOptString(base + p.Image.Thumbnail),
			Mobile:    oas.NewOptString(base + p.Image.Mobile),
			Tablet:    oas.NewOptString(base + p.Image.Tablet),
			Desktop:   oas.NewOptString(base + p.Image.Desktop),
		}),
	}
}
