package repository

import (
	"context"
	"fmt"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/xenking/oolio-kart-challenge/internal/domain/product"
)

const (
	listProductsSQL = `SELECT id, name, price, category, image_thumbnail, image_mobile, image_tablet, image_desktop
		FROM products ORDER BY id`

	getProductByIDSQL = `SELECT id, name, price, category, image_thumbnail, image_mobile, image_tablet, image_desktop
		FROM products WHERE id = $1`

	getProductsByIDsSQL = `SELECT id, name, price, category, image_thumbnail, image_mobile, image_tablet, image_desktop
		FROM products WHERE id = ANY($1)`
)

var _ product.Repository = (*ProductRepository)(nil)

// ProductRepository implements product.Repository backed by PostgreSQL.
type ProductRepository struct {
	pool *pgxpool.Pool
}

// NewProductRepository returns a ProductRepository that uses the given pool.
func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

// List returns all products from the catalog ordered by ID.
func (r *ProductRepository) List(ctx context.Context) ([]product.Product, error) {
	rows, err := r.pool.Query(ctx, listProductsSQL)
	if err != nil {
		return nil, fmt.Errorf("listing products: %w", err)
	}
	return pgx.CollectRows(rows, scanProduct)
}

// GetByID returns a single product by its identifier.
func (r *ProductRepository) GetByID(ctx context.Context, id string) (*product.Product, error) {
	rows, err := r.pool.Query(ctx, getProductByIDSQL, id)
	if err != nil {
		return nil, fmt.Errorf("getting product %q: %w", id, err)
	}

	p, err := pgx.CollectExactlyOneRow(rows, scanProduct)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, product.ErrNotFound
		}
		return nil, fmt.Errorf("getting product %q: %w", id, err)
	}
	return &p, nil
}

// GetByIDs returns products matching any of the given IDs.
func (r *ProductRepository) GetByIDs(ctx context.Context, ids []string) ([]product.Product, error) {
	rows, err := r.pool.Query(ctx, getProductsByIDsSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("getting products by ids: %w", err)
	}
	return pgx.CollectRows(rows, scanProduct)
}

func scanProduct(row pgx.CollectableRow) (product.Product, error) {
	var (
		p     product.Product
		price decimal.Decimal
	)
	err := row.Scan(
		&p.ID, &p.Name, &price, &p.Category,
		&p.Image.Thumbnail, &p.Image.Mobile, &p.Image.Tablet, &p.Image.Desktop,
	)
	p.Price = price
	return p, err
}
