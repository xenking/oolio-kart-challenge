package auth

import "context"

// APIKeyInfo holds the identity and permission data for a validated API key.
type APIKeyInfo struct {
	ID      string
	KeyHash string
	Name    string
	Scopes  []string
}

// Repository provides lookup of API keys by their HMAC hash.
type Repository interface {
	FindByHash(ctx context.Context, hash string) (*APIKeyInfo, error)
}
