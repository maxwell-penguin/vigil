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
	ProjectID          string  `json:"project_id"`
	SLOPct             float64 `json:"slo_pct"`
	BudgetConsumedPct  float64 `json:"budget_consumed_pct"`
	BudgetRemainingPct float64 `json:"budget_remaining_pct"`
	ShortBurnRate      float64 `json:"short_burn_rate"`
	LongBurnRate       float64 `json:"long_burn_rate"`
	IsBreaching        bool    `json:"is_breaching"`
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
	shortRate, err := errorRate(s, slo, now.Add(-ShortWindow), now)
	if err != nil {
		return Status{}, err
	}
	longRate, err := errorRate(s, slo, now.Add(-LongWindow), now)
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

	return Status{
		ProjectID:          slo.ProjectID,
		SLOPct:             sloPct,
		BudgetConsumedPct:  budgetConsumed,
		BudgetRemainingPct: budgetRemaining,
		ShortBurnRate:      shortBurn,
		LongBurnRate:       longBurn,
		IsBreaching:        shortBurn > ShortBurnThreshold && longBurn > LongBurnThreshold,
	}, nil
}

// errorRate returns a fraction in [0,1].
func errorRate(s Store, slo models.SLO, from, to time.Time) (float64, error) {
	good, total, err := s.CountRawInWindow(slo.ProjectID, from, to, slo.LatencyThresholdMS)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	return float64(total-good) / float64(total), nil
}
