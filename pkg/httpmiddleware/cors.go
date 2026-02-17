package httpmiddleware

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig configures the CORS middleware behaviour.
type CORSConfig struct {
	// AllowOrigins is a list of origins that are allowed to make cross-origin
	// requests. An empty list or the single entry "*" means all origins are
	// allowed.
	AllowOrigins []string

	// AllowMethods lists the HTTP methods clients may use in actual requests.
	// Defaults to "GET, POST, PUT, DELETE, OPTIONS" when empty.
	AllowMethods []string

	// AllowHeaders lists the request headers clients may use.
	// If empty, the middleware echoes back the Access-Control-Request-Headers
	// from the preflight request.
	AllowHeaders []string

	// ExposeHeaders lists response headers the browser is allowed to access.
	ExposeHeaders []string

	// AllowCredentials indicates whether the response to a request can be
	// exposed when the credentials flag is true. When true, the wildcard
	// origin "*" must not be used — the middleware echoes the specific origin.
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) preflight results can be cached.
	// A zero value omits the header; a negative value sends "0".
	MaxAge int
}

// CORS returns a middleware that handles Cross-Origin Resource Sharing.
//
// It follows the patterns from gofiber/fiber's CORS middleware:
//   - Case-insensitive origin matching with original-case echo-back
//   - Proper Vary header handling to prevent CDN cache poisoning
//   - Preflight detection via Access-Control-Request-Method header
//   - Support for credentials and expose-headers
func CORS(cfg CORSConfig) Middleware {
	allowAll := len(cfg.AllowOrigins) == 0
	allowed := make(map[string]string, len(cfg.AllowOrigins)) // lowercase -> original
	for _, o := range cfg.AllowOrigins {
		if o == "*" {
			allowAll = true
			break
		}
		allowed[strings.ToLower(o)] = o
	}

	if cfg.AllowCredentials && allowAll {
		// Credentials + wildcard is forbidden by the spec.
		// Fall back to echoing the specific origin.
		allowAll = false
	}

	allowMethods := strings.Join(cfg.AllowMethods, ", ")
	if allowMethods == "" {
		allowMethods = "GET, POST, PUT, DELETE, OPTIONS"
	}
	allowHeaders := strings.Join(cfg.AllowHeaders, ", ")
	exposeHeaders := strings.Join(cfg.ExposeHeaders, ", ")

	maxAge := ""
	if cfg.MaxAge > 0 {
		maxAge = strconv.Itoa(cfg.MaxAge)
	} else if cfg.MaxAge < 0 {
		maxAge = "0"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// No Origin header → outside CORS scope, but vary on Origin for
			// caches so a later CORS request doesn't get a stale response.
			if origin == "" {
				if !allowAll {
					w.Header().Add("Vary", "Origin")
				}
				next.ServeHTTP(w, r)
				return
			}

			// Determine the Access-Control-Allow-Origin value.
			allowOrigin := matchOrigin(origin, allowAll, allowed)

			// Preflight: OPTIONS with Access-Control-Request-Method header.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				// Vary on preflight-specific headers to prevent cache poisoning.
				w.Header().Add("Vary", "Origin")
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")

				if allowOrigin == "" {
					// Origin not allowed — let it fall through to 204 with no CORS headers.
					w.WriteHeader(http.StatusNoContent)
					return
				}

				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)

				if allowHeaders != "" {
					w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				} else if rh := r.Header.Get("Access-Control-Request-Headers"); rh != "" {
					// Echo back the requested headers (Fiber pattern).
					w.Header().Set("Access-Control-Allow-Headers", rh)
				}

				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if maxAge != "" {
					w.Header().Set("Access-Control-Max-Age", maxAge)
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Simple / actual CORS request.
			if !allowAll {
				w.Header().Add("Vary", "Origin")
			}

			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if exposeHeaders != "" {
					w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// matchOrigin returns the value to use for Access-Control-Allow-Origin.
// It returns "" if the origin is not allowed.
func matchOrigin(origin string, allowAll bool, allowed map[string]string) string {
	if allowAll {
		return "*"
	}
	// Case-insensitive lookup, but echo original-case value from config.
	if orig, ok := allowed[strings.ToLower(origin)]; ok {
		return orig
	}
	return ""
}
