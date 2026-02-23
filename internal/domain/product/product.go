package product

import (
	"context"

	"github.com/go-faster/errors"
	"github.com/shopspring/decimal"
)

// ErrNotFound is returned when a requested product does not exist.
var ErrNotFound = errors.New("product not found")

// Product represents a catalog item available for purchase.
type Product struct {
	ID       string
	Name     string
	Price    decimal.Decimal
	Category string
	Image    Image
}

// Image holds responsive image URLs for a product.
type Image struct {
	Thumbnail string
	Mobile    string
	Tablet    string
	Desktop   string
}

// Repository defines read operations for the product catalog.
type Repository interface {
	List(ctx context.Context) ([]Product, error)
	GetByID(ctx context.Context, id string) (*Product, error)
	GetByIDs(ctx context.Context, ids []string) ([]Product, error)
}
