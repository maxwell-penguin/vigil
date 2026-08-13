// Package ratelimit caps POST /ingest at a fixed rate per project, so one
// misconfigured SDK can't overwhelm the database for every other project.
package ratelimit

import (
	"encoding/json"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

const (
	eventsPerSecond = 100
	burst           = 500
)

var (
	mu       sync.Mutex
	limiters = make(map[string]*rate.Limiter)
)

// GetLimiter returns the limiter for projectID, creating one on first call.
func GetLimiter(projectID string) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()
	l, ok := limiters[projectID]
	if !ok {
		l = rate.NewLimiter(eventsPerSecond, burst)
		limiters[projectID] = l
	}
	return l
}

// Middleware rate-limits requests per project_id query param. Requests
// without a project_id are passed through unlimited — the wrapped handler
// is responsible for rejecting those itself.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := r.URL.Query().Get("project_id")
		if projectID == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !GetLimiter(projectID).Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":      "rate limit exceeded",
				"project_id": projectID,
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
