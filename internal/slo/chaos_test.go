package slo

import (
	"testing"
	"time"

	"vigil/internal/models"
)

// fakeStore is an in-memory slo.Store double for chaos testing. Unlike the
// real sqlite-backed storage.Store used elsewhere in this package's tests,
// it lets a test inject arbitrary (good, total, err) results per call and
// simulate conditions a real store can't easily produce on demand: a
// transient DB failure, or a data gap in one window but not another.
type fakeStore struct {
	// windowResults is keyed by window size (to.Sub(from)). slo.go always
	// calls CountRawInWindow with exactly ShortWindow or LongWindow, so the
	// duration alone is enough to tell the two calls apart.
	windowResults map[time.Duration]countResult

	metrics    []models.Metric
	metricsErr error

	recentEvents    []models.Event
	recentEventsErr error

	alerts []models.Alert
}

type countResult struct {
	good, total int64
	err         error
}

func newFakeStore() *fakeStore {
	return &fakeStore{windowResults: make(map[time.Duration]countResult)}
}

// setWindowResult configures what CountRawInWindow returns for calls whose
// (to - from) span equals window — pass slo.ShortWindow or slo.LongWindow.
func (f *fakeStore) setWindowResult(window time.Duration, good, total int64, err error) {
	f.windowResults[window] = countResult{good: good, total: total, err: err}
}

func (f *fakeStore) CountRawInWindow(projectID string, from, to time.Time, latencyThresholdMS int64) (good, total int64, err error) {
	r, ok := f.windowResults[to.Sub(from)]
	if !ok {
		return 0, 0, nil
	}
	return r.good, r.total, r.err
}

func (f *fakeStore) GetMetrics(projectID string, from, to time.Time) ([]models.Metric, error) {
	return f.metrics, f.metricsErr
}

func (f *fakeStore) FetchRecentRawEvents(projectID string, n int) ([]models.Event, error) {
	return f.recentEvents, f.recentEventsErr
}

func (f *fakeStore) InsertAlert(a models.Alert) error {
	f.alerts = append(f.alerts, a)
	return nil
}

func (f *fakeStore) LatestAlert(projectID string) (models.Alert, bool, error) {
	var latest models.Alert
	found := false
	for _, a := range f.alerts {
		if a.ProjectID != projectID {
			continue
		}
		if !found || a.FiredAt.After(latest.FiredAt) {
			latest = a
			found = true
		}
	}
	return latest, found, nil
}

func (f *fakeStore) MarkAlertResolved(projectID string, firedAt, resolvedAt time.Time) error {
	for i := range f.alerts {
		if f.alerts[i].ProjectID == projectID && f.alerts[i].FiredAt.Equal(firedAt) {
			f.alerts[i].ResolvedAt = resolvedAt
		}
	}
	return nil
}

func (f *fakeStore) ListAlerts(projectID string) ([]models.Alert, error) {
	var out []models.Alert
	for _, a := range f.alerts {
		if a.ProjectID == projectID {
			out = append(out, a)
		}
	}
	return out, nil
}

// TestChaos_DataGapMasksRealBreach: the long window (1h) is fed a clearly
// degraded error rate that no reasonable SLO target would allow. The short
// window (5m) is fed total=0, as if a scrape failure or agent outage left a
// gap in the most recent data rather than genuinely clean traffic.
func TestChaos_DataGapMasksRealBreach(t *testing.T) {
	s := newFakeStore()
	now := time.Now().UTC()

	sloCfg := models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 30}

	// Long window: 50 good / 100 total -> 50% error rate. Target is 99.5%,
	// so this is nowhere close to compliant on its own.
	s.setWindowResult(LongWindow, 50, 100, nil)
	// Short window: no events at all in the last 5 minutes (data gap, not
	// a genuinely quiet period).
	s.setWindowResult(ShortWindow, 0, 0, nil)

	st, err := Compute(s, sloCfg, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("computed status: %+v", st)

	// errorRate() now reports hasData alongside the rate, so Compute can
	// tell "no events in the short window" apart from "events, zero
	// errors". When the short window has no data, the short-window
	// threshold is skipped entirely and IsBreaching falls back to the long
	// window alone. Here the long window is clearly critical (50% errors
	// against a 99.5% target), so a data gap in the short window must not
	// hide that from anyone paging off IsBreaching.
	if !st.IsBreaching {
		t.Fatalf("expected IsBreaching=true (long-window breach must not be masked by a short-window data gap), got false: %+v", st)
	}
}

// TestChaos_BothWindowsHaveData_ShortHealthyLongDegraded confirms the
// data-gap fallback didn't loosen the normal two-window AND behavior: when
// the short window has real data and it's healthy, a degraded long window
// alone must still not breach.
func TestChaos_BothWindowsHaveData_ShortHealthyLongDegraded(t *testing.T) {
	s := newFakeStore()
	now := time.Now().UTC()

	sloCfg := models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 30}

	// Long window: 50 good / 100 total -> 50% error rate, well past a
	// critical burn rate on its own.
	s.setWindowResult(LongWindow, 50, 100, nil)
	// Short window: real data, healthy -> 0% error rate.
	s.setWindowResult(ShortWindow, 100, 100, nil)

	st, err := Compute(s, sloCfg, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("computed status: %+v", st)

	if st.IsBreaching {
		t.Fatalf("expected IsBreaching=false (short window has data and is healthy, AND semantics must hold), got true: %+v", st)
	}
}

// TestChaos_ClockJumpBackwardExtendsCooldown targets checker.go's cooldown
// check. It used to read:
//
//	if ok && last.ResolvedAt.IsZero() && now.Sub(last.FiredAt) < AlertCooldown {
//		continue // still within cooldown of an unresolved alert
//	}
//
// now.Sub(last.FiredAt) is a signed duration. If `now` ever went backward
// relative to the last alert's FiredAt (NTP correction, container clock
// reset, a monitoring backfill run with an old timestamp...), the
// subtraction went negative, and a negative duration always satisfied
// "< AlertCooldown" - suppressing a new alert for however long the
// backward jump was, on top of the intended cooldown, even during a real,
// ongoing breach.
//
// checkAll's cooldown check now also requires !now.Before(last.FiredAt), so
// a backward jump is never treated as "still in cooldown" - it re-fires
// immediately instead. This test documents the fixed behavior.
func TestChaos_ClockJumpBackwardExtendsCooldown(t *testing.T) {
	s := newFakeStore()
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	sloCfg := models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 30}

	// Real, ongoing breach: both windows degraded (not the data-gap case),
	// 50% errors against a 99.5% target -> burn 100 in both windows, well
	// past both thresholds. This never changes for the rest of the test,
	// so every alert (or lack of one) below is purely a cooldown-logic
	// outcome, not the breach clearing.
	s.setWindowResult(LongWindow, 50, 100, nil)
	s.setWindowResult(ShortWindow, 50, 100, nil)

	checker := NewChecker(s, []models.SLO{sloCfg}, time.Minute, nil, nil)

	// t0: first check, no prior alert -> should fire.
	checker.checkAll(t0)
	alerts, err := s.ListAlerts("p")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after checkAll(t0=%v): %d alert(s)", t0, len(alerts))
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert after first check, got %d", len(alerts))
	}
	last, ok, err := s.LatestAlert("p")
	if err != nil || !ok {
		t.Fatalf("expected a latest alert after first check: ok=%v err=%v", ok, err)
	}
	if !last.FiredAt.Equal(t0) {
		t.Fatalf("expected FiredAt=%v, got %v", t0, last.FiredAt)
	}

	// Clock jump backward: now = t0 - 10h, even though the breach never
	// stopped. now is before last.FiredAt, so the fixed check treats the
	// cooldown as not active and fires a second alert immediately, instead
	// of silently suppressing for the size of the jump.
	jumpBack := t0.Add(-10 * time.Hour)
	checker.checkAll(jumpBack)
	alerts, err = s.ListAlerts("p")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after checkAll(t0-10h=%v): %d alert(s)", jumpBack, len(alerts))
	if len(alerts) != 2 {
		t.Fatalf("expected the backward clock jump to fire a second alert immediately, got %d alert(s)", len(alerts))
	}

	// LatestAlert (both the fake here and the real sqlite-backed store, see
	// storage.Store.LatestAlert's `ORDER BY fired_at DESC`) picks the alert
	// with the greatest FiredAt value, not the most recently inserted row.
	// A1's FiredAt is still t0, which is later than jumpBack, so A1 remains
	// "latest" even after A2 fires. That means as long as `now` stays
	// behind t0, every check still finds `now` before the latest alert's
	// FiredAt and keeps firing - noisy, but never silently suppressed,
	// which is the intended direction of failure here.
	checker.checkAll(jumpBack.Add(30 * time.Minute))
	alerts, err = s.ListAlerts("p")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after checkAll(jumpBack+30m=%v): %d alert(s)", jumpBack.Add(30*time.Minute), len(alerts))
	if len(alerts) != 3 {
		t.Fatalf("expected still-before-t0 check to fire again (3 total), got %d", len(alerts))
	}

	// Now move past t0, the original FiredAt, so the true latest alert (A1)
	// is finally in the past relative to `now` again. Ordinary forward-time
	// cooldown resumes from there: +30m past t0 is still within the 1h
	// cooldown (suppressed), +61m past t0 is past it (fires).
	checker.checkAll(t0.Add(30 * time.Minute))
	alerts, err = s.ListAlerts("p")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after checkAll(t0+30m=%v): %d alert(s)", t0.Add(30*time.Minute), len(alerts))
	if len(alerts) != 3 {
		t.Fatalf("expected cooldown to suppress once past t0+30m (still 3 total), got %d", len(alerts))
	}

	checker.checkAll(t0.Add(61 * time.Minute))
	alerts, err = s.ListAlerts("p")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after checkAll(t0+61m=%v): %d alert(s)", t0.Add(61*time.Minute), len(alerts))

	// What this means in plain terms: with the fix, a backward clock jump no
	// longer hides an ongoing breach behind an inflated cooldown. Instead it
	// fires immediately, and keeps firing on every check for as long as
	// `now` stays behind the original alert's timestamp - which is noisier
	// than the intended one-alert-per-hour cadence, but it is the safe
	// direction to fail for a paging system: an operator gets paged too
	// often during a clock anomaly, not silently ignored for ~11 hours
	// during a real, ongoing outage. Once wall-clock time naturally passes
	// the original FiredAt again, normal 1-hour cooldown behavior resumes
	// exactly as before.
	if len(alerts) != 4 {
		t.Fatalf("expected cooldown to expire past t0+61m and fire again (4 total), got %d alert(s)", len(alerts))
	}
}

// TestChaos_ForwardCooldownUnaffected confirms the backward-jump fix left
// normal forward-time cooldown behavior untouched: an ongoing breach still
// gets exactly one alert per cooldown window when the clock only moves
// forward.
func TestChaos_ForwardCooldownUnaffected(t *testing.T) {
	s := newFakeStore()
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	sloCfg := models.SLO{ProjectID: "p", TargetPct: 99.5, LatencyThresholdMS: 1000, WindowDays: 30}
	s.setWindowResult(LongWindow, 50, 100, nil)
	s.setWindowResult(ShortWindow, 50, 100, nil)

	checker := NewChecker(s, []models.SLO{sloCfg}, time.Minute, nil, nil)

	checker.checkAll(t0)
	alerts, err := s.ListAlerts("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert after first check, got %d", len(alerts))
	}

	// 30 minutes later, still breaching, still within the 1h cooldown ->
	// no second alert.
	checker.checkAll(t0.Add(30 * time.Minute))
	alerts, err = s.ListAlerts("p")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after checkAll(t0+30m): %d alert(s)", len(alerts))
	if len(alerts) != 1 {
		t.Fatalf("expected cooldown to suppress at +30m (still 1 total), got %d", len(alerts))
	}

	// 61 minutes after the original alert, cooldown has expired -> fires.
	checker.checkAll(t0.Add(61 * time.Minute))
	alerts, err = s.ListAlerts("p")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after checkAll(t0+61m): %d alert(s)", len(alerts))
	if len(alerts) != 2 {
		t.Fatalf("expected cooldown to expire and fire at +61m (2 total), got %d", len(alerts))
	}
}
