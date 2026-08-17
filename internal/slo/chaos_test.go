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

	// errorRate() treats total==0 as "rate 0", the same value it would
	// return for a genuinely perfect window - it can't tell "no data" apart
	// from "no errors". IsBreaching requires BOTH short and long burn to
	// clear their thresholds (AND semantics, slo.go's Compute). So if
	// IsBreaching comes back false here, it means a data gap confined to
	// the short window is enough to fully mask a real, ongoing breach that
	// the long window is clearly showing - anyone paging off IsBreaching
	// alone would see a healthy status while 50% of requests are failing.
	// That's true regardless of whether this specific assertion is
	// "correct" or "wrong": either the AND semantics need a data-gap
	// carve-out, or on-call needs a separate "stale short window" signal.
	if st.IsBreaching {
		t.Fatalf("expected IsBreaching=false (short-window data gap masks the long-window breach), got true: %+v", st)
	}
}
