package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"github.com/go-faster/errors"

	"github.com/xenking/oolio-kart-challenge/gen/oas"
	"github.com/xenking/oolio-kart-challenge/internal/domain/auth"
)

// Compile-time check ensuring SecurityHandler satisfies the ogen interface.
var _ oas.SecurityHandler = (*SecurityHandler)(nil)

// SecurityHandler implements ogen's SecurityHandler interface, authenticating
// API requests via HMAC-SHA256 hashed API keys.
type SecurityHandler struct {
	apikeys auth.Repository
	pepper  []byte
}

// NewSecurityHandler creates a SecurityHandler with the given API key
// repository and HMAC pepper.
func NewSecurityHandler(apikeys auth.Repository, pepper []byte) *SecurityHandler {
	return &SecurityHandler{
		apikeys: apikeys,
		pepper:  pepper,
	}
}

// HandleAPIKey authenticates an incoming request by computing the HMAC-SHA256
// of the provided API key, looking it up in the repository, and performing a
// constant-time comparison to prevent timing attacks.
func (s *SecurityHandler) HandleAPIKey(ctx context.Context, _ oas.OperationName, t oas.APIKey) (context.Context, error) {
	mac := hmac.New(sha256.New, s.pepper)
	mac.Write([]byte(t.APIKey))
	hash := mac.Sum(nil)
	hexHash := hex.EncodeToString(hash)

	info, err := s.apikeys.FindByHash(ctx, hexHash)
	if err != nil {
		return ctx, errors.New("unauthorized")
	}

	// Constant-time comparison guards against timing side-channels even though
	// the lookup already succeeded — the stored hash could differ from what
	// we computed if the repository returns a stale/wrong row.
	storedBytes, err := hex.DecodeString(info.KeyHash)
	if err != nil {
		return ctx, errors.New("unauthorized")
	}

	if subtle.ConstantTimeCompare(hash, storedBytes) != 1 {
		return ctx, errors.New("unauthorized")
	}

	return ctx, nil
}
