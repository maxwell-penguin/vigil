package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"vigil/internal/models"
)

// ponytail: one self-check for the non-trivial path (downsample).
// Insert older/newer events, run Downsample, assert old ones aggregated + deleted.
func TestDownsampleRollsUpAndDeletes(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	// Truncate to a minute boundary so old, old+1s, old+2s below can never
	// straddle a 1m bucket edge — without this the test is flaky depending
	// on what second it happens to run at.
	old := now.Truncate(time.Minute).Add(-48 * time.Hour) // must roll into 1m
	fresh := now.Add(-1 * time.Minute)                    // must stay raw

	// InsertEvent only queues for the async writer; StartWriter isn't
	// running in this test, so seed directly via InsertEventsBatch instead.
	var events []models.Event
	for i := 0; i < 3; i++ {
		events = append(events, models.Event{
			ProjectID: "p", Timestamp: old.Add(time.Duration(i) * time.Second),
			LatencyMS: 100, StatusCode: 200, Error: false,
		})
	}
	events = append(events,
		models.Event{ProjectID: "p", Timestamp: old, LatencyMS: 500, StatusCode: 500, Error: true},
		models.Event{ProjectID: "p", Timestamp: fresh, LatencyMS: 50, StatusCode: 200, Error: false},
	)
	if err := s.InsertEventsBatch(events); err != nil {
		t.Fatal(err)
	}

	if err := s.Downsample(now); err != nil {
		t.Fatal(err)
	}

	var rawCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM raw_events`).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if rawCount != 1 {
		t.Fatalf("want 1 raw row remaining, got %d", rawCount)
	}

	ms, err := s.GetMetrics("p", old.Add(-time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 {
		t.Fatalf("want 1 aggregated bucket, got %d", len(ms))
	}
	if ms[0].TotalEvents != 4 || ms[0].GoodEvents != 3 {
		t.Fatalf("want good=3 total=4, got good=%d total=%d", ms[0].GoodEvents, ms[0].TotalEvents)
	}
	if ms[0].Resolution != "1m" {
		t.Fatalf("want 1m, got %s", ms[0].Resolution)
	}
}

// ponytail: one self-check for the writer goroutine — below eventBatchSize,
// so this only lands once the eventFlushInterval ticker fires, proving the
// "whichever comes first" timer path actually works (the size-triggered
// path is implicitly covered by every other test that seeds >100 events).
func TestStartWriterFlushesOnTimer(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.StartWriter(ctx)

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if err := s.InsertEvent(models.Event{
			ProjectID: "p", Timestamp: now, LatencyMS: 50, StatusCode: 200, Error: false,
		}); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM raw_events`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 5 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("events not flushed within deadline, got %d rows", count)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
