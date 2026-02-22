package httpmiddleware

import (
	"net/http"

	"github.com/go-faster/sdk/zctx"
	"go.uber.org/zap"
)

// Recovery returns a middleware that recovers from panics, logs them with a
// stack trace, and responds with 500 Internal Server Error.
func Recovery() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					lg := zctx.From(r.Context())
					lg.Error("panic recovered",
						zap.Any("panic", rec),
						zap.Stack("stack"),
					)
					w.Header().Set("Connection", "close")
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
