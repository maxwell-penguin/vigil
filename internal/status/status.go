// Package status serves a public, self-contained HTML status page per
// project — no JS, no external assets, safe to link to directly.
package status

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"vigil/internal/models"
	"vigil/internal/slo"
	"vigil/internal/storage"
)

const (
	uptimeDays = 90
	repoURL    = "https://github.com/maxwell-penguin/vigil"
)

type dayCell struct {
	Date  string
	Color string
}

type incidentRow struct {
	Date              string
	Duration          string
	BudgetConsumedPct float64
}

type pageData struct {
	ProjectID          string
	StatusText         string
	StatusColor        string
	SLOPct             float64
	TargetPct          float64
	Uptime30           float64
	BudgetRemainingPct float64
	Days               []dayCell
	Incidents          []incidentRow
	RepoURL            string
}

// Handler serves GET /status/{project_id}: a public status page built from
// the same storage layer and SLO engine the JSON API uses.
func Handler(store *storage.Store, slos []models.SLO) http.HandlerFunc {
	byID := make(map[string]models.SLO, len(slos))
	for _, s := range slos {
		byID[s.ProjectID] = s
	}

	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("project_id")
		cfg, ok := byID[id]
		if !ok {
			http.Error(w, "no SLO defined for project", http.StatusNotFound)
			return
		}

		now := time.Now()

		st, err := slo.Compute(store, cfg, now)
		if err != nil {
			log.Printf("status %s: compute: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}

		good30, total30, err := windowGoodTotal(store, id, now.Add(-30*24*time.Hour), now, cfg.LatencyThresholdMS, now)
		if err != nil {
			log.Printf("status %s: 30d uptime: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}
		uptime30 := 100.0
		if total30 > 0 {
			uptime30 = float64(good30) / float64(total30) * 100
		}

		days, err := buildDailyUptime(store, id, cfg.LatencyThresholdMS, now)
		if err != nil {
			log.Printf("status %s: daily uptime: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}

		alerts, err := store.ListAlerts(id)
		if err != nil {
			log.Printf("status %s: list alerts: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}
		incidents := make([]incidentRow, 0, 5)
		for i, a := range alerts {
			if i >= 5 {
				break
			}
			duration := "Ongoing"
			if !a.ResolvedAt.IsZero() {
				duration = formatDuration(a.ResolvedAt.Sub(a.FiredAt))
			}
			incidents = append(incidents, incidentRow{
				Date:              a.FiredAt.Format("Jan 2, 2006"),
				Duration:          duration,
				BudgetConsumedPct: a.BudgetConsumedPct,
			})
		}

		statusText, statusColor := "Operational", "#16a34a"
		switch {
		case st.IsBreaching:
			statusText, statusColor = "Outage", "#dc2626"
		case st.ShortBurnRate > 1.0 || st.LongBurnRate > 1.0:
			statusText, statusColor = "Degraded", "#d97706"
		}

		data := pageData{
			ProjectID:          id,
			StatusText:         statusText,
			StatusColor:        statusColor,
			SLOPct:             st.SLOPct,
			TargetPct:          cfg.TargetPct,
			Uptime30:           uptime30,
			BudgetRemainingPct: st.BudgetRemainingPct,
			Days:               days,
			Incidents:          incidents,
			RepoURL:            repoURL,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("status %s: render: %v", id, err)
		}
	}
}

// windowGoodTotal sums good/total events over [from, to), pulling from
// raw_events where the window falls inside the last 24h (Downsample's
// retention cutoff) and from aggregated_metrics for everything older.
func windowGoodTotal(store *storage.Store, projectID string, from, to time.Time, thresholdMS int64, now time.Time) (int64, int64, error) {
	rawCutoff := now.Add(-24 * time.Hour)
	var good, total int64

	if to.After(rawCutoff) {
		rawFrom := from
		if rawFrom.Before(rawCutoff) {
			rawFrom = rawCutoff
		}
		g, t, err := store.CountRawInWindow(projectID, rawFrom, to, thresholdMS)
		if err != nil {
			return 0, 0, err
		}
		good += g
		total += t
	}

	if from.Before(rawCutoff) {
		aggTo := to
		if aggTo.After(rawCutoff) {
			aggTo = rawCutoff
		}
		metrics, err := store.GetMetrics(projectID, from, aggTo)
		if err != nil {
			return 0, 0, err
		}
		for _, m := range metrics {
			good += m.GoodEvents
			total += m.TotalEvents
		}
	}

	return good, total, nil
}

// buildDailyUptime returns uptimeDays rolling 24h windows, oldest first, so
// the grid template can render them left-to-right with newest on the right.
func buildDailyUptime(store *storage.Store, projectID string, thresholdMS int64, now time.Time) ([]dayCell, error) {
	cells := make([]dayCell, 0, uptimeDays)
	for offset := uptimeDays - 1; offset >= 0; offset-- {
		to := now.Add(-time.Duration(offset) * 24 * time.Hour)
		from := to.Add(-24 * time.Hour)
		good, total, err := windowGoodTotal(store, projectID, from, to, thresholdMS, now)
		if err != nil {
			return nil, err
		}
		cells = append(cells, dayCell{
			Date:  to.Format("2006-01-02"),
			Color: uptimeColor(good, total),
		})
	}
	return cells, nil
}

func uptimeColor(good, total int64) string {
	if total == 0 {
		return "#e5e7eb"
	}
	pct := float64(good) / float64(total) * 100
	switch {
	case pct > 99.5:
		return "#16a34a"
	case pct >= 95:
		return "#d97706"
	default:
		return "#dc2626"
	}
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

var tmpl = template.Must(template.New("status").Parse(pageTemplate))

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.ProjectID}} status</title>
<style>
  :root { color-scheme: light; }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
    background: #ffffff;
    color: #111827;
  }
  .wrap { max-width: 640px; margin: 0 auto; padding: 48px 24px; }
  h1 { font-size: 20px; font-weight: 600; margin: 0 0 24px; }
  .status-row {
    display: flex; align-items: center; gap: 10px;
    padding: 20px; border: 1px solid #e5e7eb; border-radius: 8px;
    margin-bottom: 24px;
  }
  .dot { width: 14px; height: 14px; border-radius: 50%; background: {{.StatusColor}}; flex-shrink: 0; }
  .status-text { font-size: 16px; font-weight: 600; color: {{.StatusColor}}; }
  .stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin-bottom: 32px; }
  .stat { border: 1px solid #e5e7eb; border-radius: 8px; padding: 14px; }
  .stat .label { font-size: 11px; text-transform: uppercase; letter-spacing: .05em; color: #6b7280; margin-bottom: 6px; }
  .stat .value { font-size: 22px; font-weight: 600; font-variant-numeric: tabular-nums; }
  .section-title {
    font-size: 13px; font-weight: 600; color: #6b7280;
    text-transform: uppercase; letter-spacing: .05em; margin: 32px 0 12px;
  }
  .grid {
    display: grid; grid-template-rows: repeat(7, 10px);
    grid-auto-flow: column; grid-auto-columns: 10px; gap: 2px;
    overflow-x: auto; padding-bottom: 4px;
  }
  .cell { width: 10px; height: 10px; border-radius: 2px; }
  .legend { display: flex; gap: 14px; margin-top: 8px; font-size: 12px; color: #6b7280; align-items: center; }
  .legend .sw { width: 10px; height: 10px; border-radius: 2px; display: inline-block; margin-right: 4px; vertical-align: middle; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th, td { text-align: left; padding: 8px 4px; border-bottom: 1px solid #e5e7eb; }
  th { color: #6b7280; font-weight: 500; font-size: 11px; text-transform: uppercase; letter-spacing: .05em; }
  .empty { color: #9ca3af; font-size: 13px; padding: 12px 0; }
  footer { margin-top: 48px; padding-top: 16px; border-top: 1px solid #e5e7eb; font-size: 12px; color: #9ca3af; text-align: center; }
  footer a { color: #16a34a; text-decoration: none; }
  footer a:hover { text-decoration: underline; }
</style>
</head>
<body>
<div class="wrap">
  <h1>{{.ProjectID}}</h1>

  <div class="status-row">
    <span class="dot"></span>
    <span class="status-text">{{.StatusText}}</span>
  </div>

  <div class="stats">
    <div class="stat">
      <div class="label">SLO</div>
      <div class="value">{{printf "%.2f" .SLOPct}}%</div>
      <div class="label">target {{printf "%.1f" .TargetPct}}%</div>
    </div>
    <div class="stat">
      <div class="label">30-day uptime</div>
      <div class="value">{{printf "%.2f" .Uptime30}}%</div>
    </div>
    <div class="stat">
      <div class="label">Budget remaining</div>
      <div class="value">{{printf "%.1f" .BudgetRemainingPct}}%</div>
    </div>
  </div>

  <div class="section-title">Last 90 days</div>
  <div class="grid">
    {{range .Days}}<div class="cell" style="background:{{.Color}}" title="{{.Date}}"></div>{{end}}
  </div>
  <div class="legend">
    <span><span class="sw" style="background:#16a34a"></span>Operational</span>
    <span><span class="sw" style="background:#d97706"></span>Degraded</span>
    <span><span class="sw" style="background:#dc2626"></span>Outage</span>
    <span><span class="sw" style="background:#e5e7eb"></span>No data</span>
  </div>

  <div class="section-title">Recent incidents</div>
  {{if .Incidents}}
  <table>
    <thead><tr><th>Date</th><th>Duration</th><th>Budget consumed</th></tr></thead>
    <tbody>
      {{range .Incidents}}<tr><td>{{.Date}}</td><td>{{.Duration}}</td><td>{{printf "%.1f" .BudgetConsumedPct}}%</td></tr>{{end}}
    </tbody>
  </table>
  {{else}}
  <div class="empty">No incidents in recorded history.</div>
  {{end}}

  <footer>Powered by <a href="{{.RepoURL}}" target="_blank" rel="noreferrer">Vigil</a></footer>
</div>
</body>
</html>
`
