// Package ratelimit caps POST /ingest at a fixed rate per project, so one
// misconfigured SDK can't overwhelm the database for every other project.
package ratelimit

import (
	"container/list"
	"encoding/json"
	"net/http"
	"sync"

	"golang.org/x/time/rate"

	"vigil/internal/selfmetrics"
)

const (
	eventsPerSecond = 100
	burst           = 500

	// maxTrackedProjects bounds the LRU cache of per-project limiters.
	// project_id comes straight from an untrusted query param, so without a
	// cap a client varying it per request grows limiters unboundedly. 10k
	// comfortably covers any real deployment's active project count (each
	// entry is a tiny rate.Limiter, so even 10k costs well under a MB)
	// while still capping worst-case memory from abusive traffic.
	maxTrackedProjects = 10000
)

// lruEntry is the value stored in each list.Element, letting eviction look
// up which map key to delete without a reverse index.
type lruEntry struct {
	projectID string
	limiter   *rate.Limiter
}

var (
	mu       sync.Mutex
	limiters = make(map[string]*list.Element)
	lru      = list.New() // front = most recently used, back = least
)

// GetLimiter returns the limiter for projectID, creating one on first call.
// Tracked projects are bounded to maxTrackedProjects via LRU eviction: every
// call (hit or miss) moves projectID to the front, and a miss past the cap
// evicts the back entry first.
func GetLimiter(projectID string) *rate.Limiter {
	mu.Lock()
	defer mu.Unlock()

	if el, ok := limiters[projectID]; ok {
		lru.MoveToFront(el)
		return el.Value.(*lruEntry).limiter
	}

	if lru.Len() >= maxTrackedProjects {
		oldest := lru.Back()
		lru.Remove(oldest)
		delete(limiters, oldest.Value.(*lruEntry).projectID)
		selfmetrics.RateLimiterEvictions.Add(1)
	}

	l := rate.NewLimiter(eventsPerSecond, burst)
	el := lru.PushFront(&lruEntry{projectID: projectID, limiter: l})
	limiters[projectID] = el
	selfmetrics.RateLimiterTrackedProjects.Store(int64(lru.Len()))
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
