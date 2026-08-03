package demo

import (
	"path/filepath"
	"testing"
	"time"

	"vigil/internal/storage"
)

// ponytail: one self-check covering the two things worth getting wrong here —
// exact row counts (off-by-one in the loop bounds) and idempotency (re-seed
// must replace, not pile up).
func TestSeedIsExactAndIdempotent(t *testing.T) {
	s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	if err := Seed(s, now); err != nil {
		t.Fatal(err)
	}

	raw, err := s.FetchRecentRawEvents(ProjectID, 100000)
	if err != nil {
		t.Fatal(err)
	}
	wantRaw := int(rawWindow / rawInterval)
	if len(raw) != wantRaw {
		t.Fatalf("raw events: want %d, got %d", wantRaw, len(raw))
	}

	metrics, err := s.GetMetrics(ProjectID, now.Add(-8*24*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	// [now-7d, now-24h) truncated to the hour = 6 days of hourly buckets.
	wantBuckets := (historyDays - 1) * 24
	if len(metrics) != wantBuckets {
		t.Fatalf("aggregated buckets: want %d, got %d", wantBuckets, len(metrics))
	}

	alerts, err := s.ListAlerts(ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("alerts: want 1, got %d", len(alerts))
	}
	if alerts[0].ResolvedAt.IsZero() {
		t.Fatalf("expected the seeded breach to already be resolved")
	}

	// Re-seeding must replace, not accumulate.
	if err := Seed(s, now); err != nil {
		t.Fatal(err)
	}
	raw2, err := s.FetchRecentRawEvents(ProjectID, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw2) != wantRaw {
		t.Fatalf("re-seed raw events: want %d, got %d (data piled up)", wantRaw, len(raw2))
	}
	alerts2, err := s.ListAlerts(ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts2) != 1 {
		t.Fatalf("re-seed alerts: want 1, got %d (data piled up)", len(alerts2))
	}
}
