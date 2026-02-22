package postgres

import (
	"context"
	"fmt"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xenking/oolio-kart-challenge/gen/sqlc"
	"github.com/xenking/oolio-kart-challenge/internal/domain/auth"
)

var _ auth.Repository = (*APIKeyRepository)(nil)

// APIKeyRepository provides API key lookups backed by PostgreSQL.
type APIKeyRepository struct {
	q *sqlc.Queries
}

// NewAPIKeyRepository returns an APIKeyRepository that uses the given pool.
func NewAPIKeyRepository(pool *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{q: sqlc.New(pool)}
}

// FindByHash looks up an active API key by its HMAC-SHA256 hash.
// Returns an error wrapping pgx.ErrNoRows when no matching key exists.
func (r *APIKeyRepository) FindByHash(ctx context.Context, hash string) (*auth.APIKeyInfo, error) {
	row, err := r.q.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("api key not found: %w", err)
		}
		return nil, fmt.Errorf("finding api key by hash: %w", err)
	}

	return &auth.APIKeyInfo{
		ID:      row.ID,
		KeyHash: row.KeyHash,
		Name:    row.Name,
		Scopes:  row.Scopes,
	}, nil
}
