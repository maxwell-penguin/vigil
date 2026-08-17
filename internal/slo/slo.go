package slo

import (
	"fmt"
	"time"

	"vigil/internal/models"
)

// Store is the subset of storage.Store the SLO engine needs.
type Store interface {
	CountRawInWindow(projectID string, from, to time.Time, latencyThresholdMS int64) (good, total int64, err error)
	GetMetrics(projectID string, from, to time.Time) ([]models.Metric, error)
	FetchRecentRawEvents(projectID string, n int) ([]models.Event, error)
	InsertAlert(models.Alert) error
	LatestAlert(projectID string) (models.Alert, bool, error)
	MarkAlertResolved(projectID string, firedAt, resolvedAt time.Time) error
	ListAlerts(projectID string) ([]models.Alert, error)
}

// Notifier is called when a project starts breaching its SLO.
// It returns the tracking issue number (0 if none / not applicable).
type Notifier interface {
	NotifyBreach(slo models.SLO, status Status, recentEvents []models.Event, now time.Time) (issueNumber int, err error)
}

// Status is the computed SLO state, matching the /slo/:project_id response shape.
type Status struct {
	ProjectID                string  `json:"project_id"`
	SLOPct                   float64 `json:"slo_pct"`
	BudgetConsumedPct        float64 `json:"budget_consumed_pct"`
	BudgetRemainingPct       float64 `json:"budget_remaining_pct"`
	ShortBurnRate            float64 `json:"short_burn_rate"`
	LongBurnRate             float64 `json:"long_burn_rate"`
	IsBreaching              bool    `json:"is_breaching"`
	DaysUntilBudgetExhausted float64 `json:"days_until_budget_exhausted"` // -1 if not burning, 0 if already exhausted
	BurnTrajectory           string  `json:"burn_trajectory"`             // "healthy", "warning", "critical", "exhausted"
}

const (
	ShortWindow        = 5 * time.Minute
	LongWindow         = 1 * time.Hour
	ShortBurnThreshold = 14.4
	LongBurnThreshold  = 1.0
	AlertCooldown      = 1 * time.Hour
)

// Compute the current SLO status for one project.
func Compute(s Store, slo models.SLO, now time.Time) (Status, error) {
	// SLO% over the full window from aggregated_metrics.
	// ponytail: aggregated `good` is error-only — latency threshold cannot be applied
	// retroactively to rolled-up rows. Upgrade path: extend aggregated_metrics with
	// good_events_fast/slow columns or a per-project latency threshold snapshot.
	windowStart := now.Add(-time.Duration(slo.WindowDays) * 24 * time.Hour)
	metrics, err := s.GetMetrics(slo.ProjectID, windowStart, now)
	if err != nil {
		return Status{}, fmt.Errorf("get metrics: %w", err)
	}
	var good, total int64
	for _, m := range metrics {
		good += m.GoodEvents
		total += m.TotalEvents
	}
	sloPct := 100.0
	if total > 0 {
		sloPct = float64(good) / float64(total) * 100
	}

	budgetConsumed := 0.0
	if slo.TargetPct > 0 {
		budgetConsumed = (1 - sloPct/slo.TargetPct) * 100
	}
	if budgetConsumed < 0 {
		budgetConsumed = 0
	}
	if budgetConsumed > 100 {
		budgetConsumed = 100
	}
	// ponytail: the task's `budget_remaining = error_budget - budget_consumed` mixes
	// percentage-of-budget with percentage-points and goes negative in the normal case.
	// Interpreting `budget_remaining_pct` as "% of allowed error budget still available".
	budgetRemaining := 100 - budgetConsumed

	// Burn windows both hit raw_events: the 1h window is always inside the raw
	// retention window (Downsample only rolls rows older than 24h).
	shortRate, shortHasData, err := errorRate(s, slo, now.Add(-ShortWindow), now)
	if err != nil {
		return Status{}, err
	}
	longRate, _, err := errorRate(s, slo, now.Add(-LongWindow), now)
	if err != nil {
		return Status{}, err
	}

	// Google SRE workbook burn rate: current error rate / allowed error rate.
	// The task's `* (30days/window)` multiplier would push values ~10^3-10^4× the
	// 14.4/1.0 thresholds and never trigger; using the formula the thresholds assume.
	budgetFrac := (100 - slo.TargetPct) / 100
	var shortBurn, longBurn float64
	if budgetFrac > 0 {
		shortBurn = shortRate / budgetFrac
		longBurn = longRate / budgetFrac
	}

	// ponytail: the task's formula multiplies in an extra (1 - slo_pct/100)
	// factor, but long_burn_rate above is already error_rate / allowed_error_rate
	// — by that definition, burn_rate=1.0 exhausts the *whole* budget in exactly
	// window_days, so days-to-exhaust the *remaining* budget is just
	// (remaining_fraction * window_days) / burn_rate. The extra factor would
	// double-count the error rate and overstate the runway by roughly
	// 1/(1-slo_pct/100)x.
	daysUntilExhausted := -1.0
	trajectory := "healthy"
	switch {
	case budgetRemaining <= 0:
		daysUntilExhausted = 0
		trajectory = "exhausted"
	case longBurn > 0:
		daysUntilExhausted = (budgetRemaining / 100) * float64(slo.WindowDays) / longBurn
		switch {
		case daysUntilExhausted < 2:
			trajectory = "critical"
		case daysUntilExhausted < 7:
			trajectory = "warning"
		}
	}

	// Short window with no data at all (a monitoring gap, not genuinely
	// clean traffic) can't tell "healthy" from "unmeasured" — requiring it
	// to clear ShortBurnThreshold would let a gap silently mask a real,
	// ongoing breach the long window is clearly showing. So the short-window
	// requirement only applies when the short window actually has data;
	// otherwise IsBreaching falls back to the long window alone.
	isBreaching := longBurn > LongBurnThreshold
	if shortHasData {
		isBreaching = isBreaching && shortBurn > ShortBurnThreshold
	}

	return Status{
		ProjectID:                slo.ProjectID,
		SLOPct:                   sloPct,
		BudgetConsumedPct:        budgetConsumed,
		BudgetRemainingPct:       budgetRemaining,
		ShortBurnRate:            shortBurn,
		LongBurnRate:             longBurn,
		IsBreaching:              isBreaching,
		DaysUntilBudgetExhausted: daysUntilExhausted,
		BurnTrajectory:           trajectory,
	}, nil
}

// errorRate returns the error fraction in [0,1] for the window, and whether
// the window had any events at all. total==0 (a data gap) is not the same
// as a genuinely 0% error rate — callers that need to tell them apart use
// hasData rather than treating rate==0 as "no errors".
func errorRate(s Store, slo models.SLO, from, to time.Time) (rate float64, hasData bool, err error) {
	good, total, err := s.CountRawInWindow(slo.ProjectID, from, to, slo.LatencyThresholdMS)
	if err != nil {
		return 0, false, err
	}
	if total == 0 {
		return 0, false, nil
	}
	return float64(total-good) / float64(total), true, nil
}
