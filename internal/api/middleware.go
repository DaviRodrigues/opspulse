package api

import (
	"net/http"
	"strings"
)

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAll := false
	allowedSet := make(map[string]struct{}, len(allowedOrigins))
	for _, raw := range allowedOrigins {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			continue
		}
		if origin == "*" {
			allowAll = true
			continue
		}
		allowedSet[origin] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Vary", "Origin")

			origin := strings.TrimSpace(r.Header.Get("Origin"))
			allowed := false
			if origin != "" {
				if allowAll {
					allowed = true
				} else {
					_, allowed = allowedSet[origin]
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Requested-With")
			}

			if r.Method == http.MethodOptions {
				if origin != "" && !allowed {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
