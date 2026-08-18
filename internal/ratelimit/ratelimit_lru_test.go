package ratelimit

import (
	"fmt"
	"sync"
	"testing"

	"vigil/internal/selfmetrics"
)

// TestGetLimiter_BoundsMemoryUnderCardinalityPressure proves the LRU cap
// actually bounds cache size, not just that the code compiles: pushing
// maxTrackedProjects+5000 distinct project_ids through GetLimiter must
// leave exactly maxTrackedProjects tracked, evicting the rest.
//
// The eviction delta is computed relative to a captured baseline rather
// than hardcoded as 5000, because internal/ratelimit's map/list state is
// package-level and shared across every test in this binary (and any
// future -shuffle=on run) — other tests may have already inserted a
// handful of project_ids before this one runs. Regardless of that
// baseline, inserting maxTrackedProjects+5000 brand-new unique ids always
// overflows the cache by exactly 5000 past whatever was already tracked,
// which is what actually proves the bound: it isn't "starts empty", it's
// "never grows past the cap no matter what was there before".
func TestGetLimiter_BoundsMemoryUnderCardinalityPressure(t *testing.T) {
	baseTracked := selfmetrics.RateLimiterTrackedProjects.Load()
	baseEvictions := selfmetrics.RateLimiterEvictions.Load()

	const extra = 5000
	for i := 0; i < maxTrackedProjects+extra; i++ {
		GetLimiter(fmt.Sprintf("cardinality-%d", i))
	}

	if got := selfmetrics.RateLimiterTrackedProjects.Load(); got != maxTrackedProjects {
		t.Fatalf("tracked projects = %d, want exactly %d (cap not holding)", got, maxTrackedProjects)
	}

	wantEvictions := baseTracked + extra
	if got := selfmetrics.RateLimiterEvictions.Load() - baseEvictions; got != wantEvictions {
		t.Fatalf("evictions increased by %d, want %d (baseline tracked=%d + %d new-beyond-capacity inserts)",
			got, wantEvictions, baseTracked, extra)
	}
}

// TestGetLimiter_RecentlyUsedSurvivesEviction proves eviction order is
// actually LRU, not insertion order or something else: an entry that keeps
// getting touched must never be evicted even while the cache churns through
// a full cap's worth of other entries around it.
func TestGetLimiter_RecentlyUsedSurvivesEviction(t *testing.T) {
	keep := GetLimiter("keep-me")

	for i := 0; i < maxTrackedProjects; i++ {
		GetLimiter(fmt.Sprintf("filler-%d", i))
		if i%100 == 0 {
			GetLimiter("keep-me") // refresh recency
		}
	}

	if got := GetLimiter("keep-me"); got != keep {
		t.Fatalf("keep-me limiter pointer changed (was evicted and recreated): got %p, want %p", got, keep)
	}
}

// TestGetLimiter_ConcurrentAccessNoRace exercises GetLimiter from many
// goroutines with a mix of shared and unique project_ids. Its job is to be
// run under -race: it doesn't assert much beyond "no panic" and "the cap
// still holds under concurrent pressure".
func TestGetLimiter_ConcurrentAccessNoRace(t *testing.T) {
	const goroutines = 50
	const itersPerGoroutine = 200
	sharedIDs := []string{"shared-a", "shared-b", "shared-c"}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < itersPerGoroutine; i++ {
				id := fmt.Sprintf("concurrent-%d-%d", g, i)
				if i%3 == 0 {
					id = sharedIDs[i%len(sharedIDs)]
				}
				if GetLimiter(id) == nil {
					t.Errorf("GetLimiter(%q) returned nil", id)
				}
			}
		}(g)
	}
	wg.Wait()

	if got := selfmetrics.RateLimiterTrackedProjects.Load(); got > maxTrackedProjects {
		t.Fatalf("tracked projects = %d, want <= %d", got, maxTrackedProjects)
	}
}
