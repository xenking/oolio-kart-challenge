package postgres

import (
	"context"
	"fmt"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xenking/oolio-kart-challenge/internal/dbgen"
	"github.com/xenking/oolio-kart-challenge/internal/product"
)

var _ product.Repository = (*ProductRepository)(nil)

// ProductRepository implements product.Repository backed by PostgreSQL.
type ProductRepository struct {
	q *dbgen.Queries
}

// NewProductRepository returns a ProductRepository that uses the given pool.
func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{q: dbgen.New(pool)}
}

// List returns all products from the catalog ordered by ID.
func (r *ProductRepository) List(ctx context.Context) ([]product.Product, error) {
	rows, err := r.q.ListProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing products: %w", err)
	}

	products := make([]product.Product, len(rows))
	for i, row := range rows {
		products[i] = mapProduct(row)
	}
	return products, nil
}

// GetByID returns a single product by its identifier. It returns an error
// wrapping pgx.ErrNoRows when no matching product exists.
func (r *ProductRepository) GetByID(ctx context.Context, id string) (*product.Product, error) {
	row, err := r.q.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, product.ErrNotFound
		}
		return nil, fmt.Errorf("getting product %q: %w", id, err)
	}

	p := mapProduct(row)
	return &p, nil
}

func mapProduct(row dbgen.Product) product.Product {
	return product.Product{
		ID:       row.ID,
		Name:     row.Name,
		Price:    row.Price,
		Category: row.Category,
		Image: product.Image{
			Thumbnail: row.ImageThumbnail,
			Mobile:    row.ImageMobile,
			Tablet:    row.ImageTablet,
			Desktop:   row.ImageDesktop,
		},
	}
}
