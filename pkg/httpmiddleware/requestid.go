package httpmiddleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// requestIDKey is the context key for the request ID value.
type requestIDKey struct{}

// RequestIDFromContext extracts the request ID from the context.
// It returns an empty string if no request ID is present.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// RequestID returns a middleware that ensures every request has a unique
// identifier. If the incoming request already carries a valid X-Request-ID
// header, that value is reused. Otherwise a new UUID v4 is generated.
// Incoming values are validated: they must be at most 128 bytes and contain
// only printable ASCII characters (0x20–0x7E).
//
// The request ID is:
//   - Set on the response X-Request-ID header.
//   - Stored in the request context (retrieve with RequestIDFromContext).
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if !isValidRequestID(id) {
				id = uuid.New().String()
			}

			w.Header().Set("X-Request-ID", id)

			ctx := context.WithValue(r.Context(), requestIDKey{}, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isValidRequestID checks that id is non-empty, at most 128 bytes, and
// contains only printable ASCII (0x20–0x7E).
func isValidRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i := range len(id) {
		if id[i] < 0x20 || id[i] > 0x7E {
			return false
		}
	}
	return true
}
