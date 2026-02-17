package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"github.com/go-faster/errors"

	"github.com/xenking/oolio-kart-challenge/internal/oas"
)

// HandleAPIKey authenticates an incoming request by hashing the provided API
// key, looking it up in the repository, and performing a constant-time
// comparison to prevent timing attacks.
func (h *Handler) HandleAPIKey(ctx context.Context, _ oas.OperationName, t oas.APIKey) (context.Context, error) {
	hash := sha256.Sum256([]byte(t.APIKey))
	hexHash := hex.EncodeToString(hash[:])

	info, err := h.apikeys.FindByHash(ctx, hexHash)
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

	if subtle.ConstantTimeCompare(hash[:], storedBytes) != 1 {
		return ctx, errors.New("unauthorized")
	}

	return ctx, nil
}
