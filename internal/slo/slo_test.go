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

// ponytail: one table-driven self-check covering all four trajectory states.
// SLO target 99% → budget_frac 0.01, so error_rate/0.01 gives an easy-to-pick
// long_burn_rate: 6% errors → burn 6.0 → 30d*100%/6.0 = 5 days (warning);
// 20% errors → burn 20.0 → 30d*100%/20.0 = 1.5 days (critical).
func TestComputeBurnTrajectory(t *testing.T) {
	newStore := func(t *testing.T) *storage.Store {
		s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	}
	sloCfg := models.SLO{ProjectID: "p", TargetPct: 99, LatencyThresholdMS: 1000, WindowDays: 30}

	t.Run("healthy: zero burn", func(t *testing.T) {
		s := newStore(t)
		now := time.Now().UTC()
		var events []models.Event
		for i := 0; i < 100; i++ {
			events = append(events, models.Event{
				ProjectID: "p", Timestamp: now.Add(-time.Duration(i) * time.Second),
				LatencyMS: 50, StatusCode: 200,
			})
		}
		if err := s.InsertEventsBatch(events); err != nil {
			t.Fatal(err)
		}

		st, err := Compute(s, sloCfg, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if st.LongBurnRate != 0 {
			t.Fatalf("expected zero burn, got %v", st.LongBurnRate)
		}
		if st.BurnTrajectory != "healthy" || st.DaysUntilBudgetExhausted != -1 {
			t.Fatalf("got trajectory=%q days=%v, want healthy/-1", st.BurnTrajectory, st.DaysUntilBudgetExhausted)
		}
	})

	t.Run("warning: 5 days out", func(t *testing.T) {
		s := newStore(t)
		now := time.Now().UTC()
		var events []models.Event
		for i := 0; i < 100; i++ {
			e := models.Event{ProjectID: "p", Timestamp: now.Add(-time.Duration(i) * time.Second), LatencyMS: 50, StatusCode: 200}
			if i < 6 { // 6% error rate
				e.StatusCode = 500
				e.Error = true
			}
			events = append(events, e)
		}
		if err := s.InsertEventsBatch(events); err != nil {
			t.Fatal(err)
		}

		st, err := Compute(s, sloCfg, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if st.BurnTrajectory != "warning" {
			t.Fatalf("got trajectory=%q, want warning (days=%v)", st.BurnTrajectory, st.DaysUntilBudgetExhausted)
		}
		if st.DaysUntilBudgetExhausted < 2 || st.DaysUntilBudgetExhausted >= 7 {
			t.Fatalf("days=%v outside warning range [2,7)", st.DaysUntilBudgetExhausted)
		}
	})

	t.Run("critical: 1.5 days out", func(t *testing.T) {
		s := newStore(t)
		now := time.Now().UTC()
		var events []models.Event
		for i := 0; i < 100; i++ {
			e := models.Event{ProjectID: "p", Timestamp: now.Add(-time.Duration(i) * time.Second), LatencyMS: 50, StatusCode: 200}
			if i < 20 { // 20% error rate
				e.StatusCode = 500
				e.Error = true
			}
			events = append(events, e)
		}
		if err := s.InsertEventsBatch(events); err != nil {
			t.Fatal(err)
		}

		st, err := Compute(s, sloCfg, now.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if st.BurnTrajectory != "critical" {
			t.Fatalf("got trajectory=%q, want critical (days=%v)", st.BurnTrajectory, st.DaysUntilBudgetExhausted)
		}
		if st.DaysUntilBudgetExhausted < 0 || st.DaysUntilBudgetExhausted >= 2 {
			t.Fatalf("days=%v outside critical range [0,2)", st.DaysUntilBudgetExhausted)
		}
	})

	t.Run("exhausted: budget already gone", func(t *testing.T) {
		s := newStore(t)
		now := time.Now().UTC()
		// Aggregated (not raw) metrics drive SLOPct/budget_consumed — a bucket
		// with zero good events inside the window forces budget_remaining to 0.
		if err := s.InsertMetricsBatch([]models.Metric{{
			ProjectID: "p", BucketStart: now.Add(-29 * 24 * time.Hour), Resolution: "1h",
			GoodEvents: 0, TotalEvents: 100,
		}}); err != nil {
			t.Fatal(err)
		}

		st, err := Compute(s, sloCfg, now)
		if err != nil {
			t.Fatal(err)
		}
		if st.BudgetRemainingPct != 0 {
			t.Fatalf("expected budget fully consumed, got remaining=%v", st.BudgetRemainingPct)
		}
		if st.BurnTrajectory != "exhausted" || st.DaysUntilBudgetExhausted != 0 {
			t.Fatalf("got trajectory=%q days=%v, want exhausted/0", st.BurnTrajectory, st.DaysUntilBudgetExhausted)
		}
	})
}
