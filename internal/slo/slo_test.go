package slo

import (
	"path/filepath"
	"testing"
	"time"

	"vigil/internal/models"
	"vigil/internal/storage"
)

// ponytail: one self-check exercising Compute + IsBreaching against a real store.
// Insert 100 fresh events, 30 of which are errors → error_rate 0.3.
// SLO 99% → budget_frac 0.01 → burn 30.0 in both windows → breaches.
func TestComputeBreaches(t *testing.T) {
	s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	// InsertEvent only queues for the async writer; StartWriter isn't
	// running in this test, so seed directly via InsertEventsBatch instead.
	var events []models.Event
	for i := 0; i < 100; i++ {
		e := models.Event{
			ProjectID:  "p",
			Timestamp:  now.Add(-time.Duration(i) * time.Second),
			LatencyMS:  50,
			StatusCode: 200,
			Error:      false,
		}
		if i < 30 {
			e.StatusCode = 500
			e.Error = true
		}
		events = append(events, e)
	}
	if err := s.InsertEventsBatch(events); err != nil {
		t.Fatal(err)
	}

	st, err := Compute(s, models.SLO{
		ProjectID:          "p",
		TargetPct:          99,
		LatencyThresholdMS: 1000,
		WindowDays:         30,
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	if !st.IsBreaching {
		t.Fatalf("expected breach, got status %+v", st)
	}
	if st.ShortBurnRate < 14.4 || st.LongBurnRate < 1.0 {
		t.Fatalf("burn rates below thresholds: %+v", st)
	}
	// no alert yet; feed to checker (no Notifier) to fire one.
	c := NewChecker(s, []models.SLO{{
		ProjectID: "p", TargetPct: 99, LatencyThresholdMS: 1000, WindowDays: 30,
	}}, time.Minute, nil, nil)
	c.checkAll(now.Add(time.Second))
	if _, ok, err := s.LatestAlert("p"); err != nil || !ok {
		t.Fatalf("expected alert row, ok=%v err=%v", ok, err)
	}
}
