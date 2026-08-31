package core

import (
	"crypto/subtle"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/seikaikyo/go-common/response"
)

const (
	// DefaultListenHost is the bind address used when nothing overrides it.
	// The scan API drives outbound Modbus connections, so it must not be
	// reachable from other hosts unless the operator says so explicitly.
	DefaultListenHost = "127.0.0.1"

	// DefaultTokenEnv is the environment variable read for the API token.
	DefaultTokenEnv = "EDGE_GATEWAY_API_TOKEN"

	// ListenEnv overrides the configured bind address.
	ListenEnv = "EDGE_GATEWAY_LISTEN"

	// MinTokenLength is the shortest API token accepted. Short tokens are
	// treated as unconfigured rather than silently allowed.
	MinTokenLength = 16
)

// APIToken returns the configured API token, or "" when none is usable.
func APIToken(cfg *Config) string {
	name := cfg.API.TokenEnv
	if name == "" {
		name = DefaultTokenEnv
	}
	token := strings.TrimSpace(os.Getenv(name))
	if len(token) < MinTokenLength {
		return ""
	}
	return token
}

// ListenHost resolves the bind address. Precedence: explicit flag value, then
// EDGE_GATEWAY_LISTEN, then config, then loopback.
func ListenHost(cfg *Config, flagValue string) string {
	if v := strings.TrimSpace(flagValue); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv(ListenEnv)); v != "" {
		return v
	}
	if v := strings.TrimSpace(cfg.API.Listen); v != "" {
		return v
	}
	return DefaultListenHost
}

// IsLoopbackHost reports whether the bind address keeps the server local.
func IsLoopbackHost(host string) bool {
	switch strings.TrimSpace(host) {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	}
	return false
}

// tokenAuth rejects any /api request that does not present the API token.
//
// Fail closed: when no token is configured the routes answer 503 instead of
// running unauthenticated. Comparison is constant time.
func tokenAuth(token string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				response.Err(w, http.StatusServiceUnavailable, "api authentication is not configured")
				return
			}

			presented := bearerToken(r)
			if presented == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="edge-gateway"`)
				response.Err(w, http.StatusUnauthorized, "authentication required")
				return
			}

			if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
				logger.Warn("api auth rejected", "path", r.URL.Path, "remote", r.RemoteAddr)
				w.Header().Set("WWW-Authenticate", `Bearer realm="edge-gateway"`)
				response.Err(w, http.StatusUnauthorized, "authentication required")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts the credential from Authorization or X-API-Token.
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if len(h) >= 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
		return ""
	}
	return strings.TrimSpace(r.Header.Get("X-API-Token"))
}

// requestGuard blocks browser-driven cross-site calls.
//
// Two checks: a state-changing request must declare application/json, which a
// form or a text/plain fetch cannot do without a preflight; and an Origin
// header, when present, must be same-origin or explicitly allowed.
func requestGuard(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[strings.ToLower(o)] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" {
				if !originAllowed(origin, r.Host, allowed) {
					response.Err(w, http.StatusForbidden, "origin not allowed")
					return
				}
			}

			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				if !isJSONContentType(r.Header.Get("Content-Type")) {
					response.Err(w, http.StatusUnsupportedMediaType, "content-type must be application/json")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// originAllowed accepts same-origin requests and configured origins only.
func originAllowed(origin, host string, allowed map[string]bool) bool {
	if allowed[strings.ToLower(strings.TrimSpace(origin))] {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

// isJSONContentType reports whether the media type is application/json,
// ignoring parameters such as charset.
func isJSONContentType(value string) bool {
	if value == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	return strings.EqualFold(mt, "application/json")
}
