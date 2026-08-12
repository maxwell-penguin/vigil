// Package auth gates non-public endpoints behind a single shared API key.
package auth

import (
	"encoding/json"
	"net/http"
)

// Middleware checks every request for `Authorization: Bearer {apiKey}`.
// If apiKey is empty, the returned middleware is a no-op — existing
// deployments without a key configured keep working unauthenticated.
//
// Callers decide which routes get wrapped; badge/status/healthz stay public
// simply by never being passed through this middleware.
func Middleware(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if apiKey == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+apiKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
