package slo

import (
	"math"
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

// mkEvents builds n events for a project at a fixed timestamp, the first
// `errors` of which are errors — good enough for burn rate tests, which
// only care about counts within a window, not per-event spacing.
func mkEvents(projectID string, ts time.Time, n, errors int) []models.Event {
	events := make([]models.Event, n)
	for i := range events {
		events[i] = models.Event{
			ProjectID: projectID, Timestamp: ts, LatencyMS: 50, StatusCode: 200,
		}
		if i < errors {
			events[i].StatusCode = 500
			events[i].Error = true
		}
	}
	return events
}

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// TestBurnRateRegression pins down the burn rate formula (error_rate /
// allowed_error_rate) and IsBreaching so a future edit to the scaling
// factor — like the one that was already fixed — gets caught immediately.
func TestBurnRateRegression(t *testing.T) {
	const burnTol = 0.01
	const budgetTol = 0.1

	type tc struct {
		name          string
		slo           models.SLO
		seed          func(t *testing.T, s *storage.Store, now time.Time)
		wantShort     float64
		wantLong      float64
		wantBreaching bool
		wantBudgetPct float64
	}

	cases := []tc{
		{
			name: "zero traffic: no panic, burn rate zero",
			slo:  models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 30},
			seed: func(t *testing.T, s *storage.Store, now time.Time) {
				// nothing seeded
			},
			wantShort: 0, wantLong: 0, wantBreaching: false, wantBudgetPct: 100,
		},
		{
			name: "perfect traffic: 0% errors, not breaching",
			slo:  models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 14},
			seed: func(t *testing.T, s *storage.Store, now time.Time) {
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-30*time.Second), 500, 0)); err != nil {
					t.Fatal(err)
				}
				if err := s.InsertMetricsBatch([]models.Metric{{
					ProjectID: "p", BucketStart: now.Add(-time.Hour), Resolution: "1h",
					GoodEvents: 1000, TotalEvents: 1000,
				}}); err != nil {
					t.Fatal(err)
				}
			},
			wantShort: 0, wantLong: 0, wantBreaching: false, wantBudgetPct: 100,
		},
		{
			// 0.5% errors on a 99.5% SLO: budget_frac = 0.5% too, so burn
			// rate lands exactly on 1.0 — the boundary must not breach.
			name: "exactly at SLO threshold: burn = 1.0, not breaching",
			slo:  models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 7},
			seed: func(t *testing.T, s *storage.Store, now time.Time) {
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-30*time.Second), 200, 1)); err != nil {
					t.Fatal(err)
				}
				if err := s.InsertMetricsBatch([]models.Metric{{
					ProjectID: "p", BucketStart: now.Add(-time.Hour), Resolution: "1h",
					GoodEvents: 199, TotalEvents: 200,
				}}); err != nil {
					t.Fatal(err)
				}
			},
			wantShort: 1.0, wantLong: 1.0, wantBreaching: false, wantBudgetPct: 100,
		},
		{
			// 1.2% errors against the same 1% budget_frac: burn rate must
			// clear 1.0, not sit at it.
			name: "just above threshold: burn > 1.0",
			slo:  models.SLO{ProjectID: "p", TargetPct: 99, LatencyThresholdMS: 1000, WindowDays: 90},
			seed: func(t *testing.T, s *storage.Store, now time.Time) {
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-30*time.Second), 1000, 12)); err != nil {
					t.Fatal(err)
				}
				if err := s.InsertMetricsBatch([]models.Metric{{
					ProjectID: "p", BucketStart: now.Add(-time.Hour), Resolution: "1h",
					GoodEvents: 990, TotalEvents: 1000,
				}}); err != nil {
					t.Fatal(err)
				}
			},
			wantShort: 1.2, wantLong: 1.2, wantBreaching: false, wantBudgetPct: 100,
		},
		{
			// Short window (last 5m) is hot at 80% errors; the rest of the
			// hour is clean enough to dilute the long-window rate back
			// under its threshold. AND semantics must not fire.
			name: "short window high, long window low: not breaching",
			slo:  models.SLO{ProjectID: "p", TargetPct: 95, LatencyThresholdMS: 1000, WindowDays: 60},
			seed: func(t *testing.T, s *storage.Store, now time.Time) {
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-time.Minute), 100, 80)); err != nil {
					t.Fatal(err)
				}
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-10*time.Minute), 2000, 0)); err != nil {
					t.Fatal(err)
				}
			},
			// short: 80/100 = 0.8 -> burn 16.0 (>14.4, breach)
			// long: 80/2100 = 0.038095 -> burn 0.761905 (<1.0, no breach)
			wantShort: 16.0, wantLong: 0.761905, wantBreaching: false, wantBudgetPct: 100,
		},
		{
			// Both windows run hot at 80% errors: AND semantics must fire.
			name: "both windows high: breaching",
			slo:  models.SLO{ProjectID: "p", TargetPct: 95, LatencyThresholdMS: 1000, WindowDays: 10},
			seed: func(t *testing.T, s *storage.Store, now time.Time) {
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-time.Minute), 100, 80)); err != nil {
					t.Fatal(err)
				}
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-30*time.Minute), 100, 80)); err != nil {
					t.Fatal(err)
				}
			},
			// short: 80/100 = 0.8 -> burn 16.0 (>14.4)
			// long: 160/200 = 0.8 -> burn 16.0 (>1.0)
			wantShort: 16.0, wantLong: 16.0, wantBreaching: true, wantBudgetPct: 100,
		},
		{
			// 100% errors: burn rate must stay a large-but-finite number,
			// never Inf/NaN, and budget must clamp at 0 remaining.
			name: "budget fully exhausted: bounded, no overflow",
			slo:  models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 45},
			seed: func(t *testing.T, s *storage.Store, now time.Time) {
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-30*time.Second), 100, 100)); err != nil {
					t.Fatal(err)
				}
				if err := s.InsertMetricsBatch([]models.Metric{{
					ProjectID: "p", BucketStart: now.Add(-time.Hour), Resolution: "1h",
					GoodEvents: 0, TotalEvents: 100,
				}}); err != nil {
					t.Fatal(err)
				}
			},
			// 1.0 error rate / 0.005 budget_frac = 200.0
			wantShort: 200.0, wantLong: 200.0, wantBreaching: true, wantBudgetPct: 0,
		},
		{
			// Only 3 raw events in the window — the formula must still
			// divide cleanly rather than special-casing a minimum sample.
			name: "partial window data: few events still compute correctly",
			slo:  models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 21},
			seed: func(t *testing.T, s *storage.Store, now time.Time) {
				if err := s.InsertEventsBatch(mkEvents("p", now.Add(-30*time.Second), 3, 1)); err != nil {
					t.Fatal(err)
				}
				if err := s.InsertMetricsBatch([]models.Metric{{
					ProjectID: "p", BucketStart: now.Add(-time.Hour), Resolution: "1h",
					GoodEvents: 2, TotalEvents: 3,
				}}); err != nil {
					t.Fatal(err)
				}
			},
			// (1/3) / 0.005 = 66.6667
			wantShort: 66.6667, wantLong: 66.6667, wantBreaching: true, wantBudgetPct: 67.0017,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()

			now := time.Now().UTC()
			c.seed(t, s, now)

			st, err := Compute(s, c.slo, now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}

			if math.IsNaN(st.ShortBurnRate) || math.IsInf(st.ShortBurnRate, 0) {
				t.Fatalf("ShortBurnRate not finite: %v", st.ShortBurnRate)
			}
			if math.IsNaN(st.LongBurnRate) || math.IsInf(st.LongBurnRate, 0) {
				t.Fatalf("LongBurnRate not finite: %v", st.LongBurnRate)
			}
			if !almostEqual(st.ShortBurnRate, c.wantShort, burnTol) {
				t.Errorf("ShortBurnRate = %v, want %v (±%v)", st.ShortBurnRate, c.wantShort, burnTol)
			}
			if !almostEqual(st.LongBurnRate, c.wantLong, burnTol) {
				t.Errorf("LongBurnRate = %v, want %v (±%v)", st.LongBurnRate, c.wantLong, burnTol)
			}
			if st.IsBreaching != c.wantBreaching {
				t.Errorf("IsBreaching = %v, want %v (short=%v long=%v)", st.IsBreaching, c.wantBreaching, st.ShortBurnRate, st.LongBurnRate)
			}
			if !almostEqual(st.BudgetRemainingPct, c.wantBudgetPct, budgetTol) {
				t.Errorf("BudgetRemainingPct = %v, want %v (±%v)", st.BudgetRemainingPct, c.wantBudgetPct, budgetTol)
			}
		})
	}
}
