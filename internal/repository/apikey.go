package repository

import (
	"context"
	"fmt"

	"github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/xenking/oolio-kart-challenge/internal/domain/auth"
)

const getAPIKeyByHashSQL = `SELECT id, key_hash, name, scopes
	FROM api_keys WHERE key_hash = $1 AND active = TRUE`

var _ auth.Repository = (*APIKeyRepository)(nil)

// APIKeyRepository provides API key lookups backed by PostgreSQL.
type APIKeyRepository struct {
	pool *pgxpool.Pool
}

// NewAPIKeyRepository returns an APIKeyRepository that uses the given pool.
func NewAPIKeyRepository(pool *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{pool: pool}
}

// FindByHash looks up an active API key by its HMAC-SHA256 hash.
func (r *APIKeyRepository) FindByHash(ctx context.Context, hash string) (*auth.APIKeyInfo, error) {
	var info auth.APIKeyInfo
	err := r.pool.QueryRow(ctx, getAPIKeyByHashSQL, hash).Scan(
		&info.ID, &info.KeyHash, &info.Name, &info.Scopes,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("api key not found: %w", err)
		}
		return nil, fmt.Errorf("finding api key by hash: %w", err)
	}
	return &info, nil
}
