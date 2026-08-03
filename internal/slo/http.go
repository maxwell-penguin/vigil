package slo

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"vigil/internal/models"
)

// StatusHandler serves GET /slo/{project_id}.
// Uses Go 1.22 net/http path patterns.
func StatusHandler(store Store, slos []models.SLO) http.HandlerFunc {
	byID := make(map[string]models.SLO, len(slos))
	for _, s := range slos {
		byID[s.ProjectID] = s
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("project_id")
		s, ok := byID[id]
		if !ok {
			http.Error(w, "no SLO defined for project", http.StatusNotFound)
			return
		}
		st, err := Compute(store, s, time.Now())
		if err != nil {
			log.Printf("slo status %s: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st)
	}
}

// Incident is one past breach, shaped for the /incidents/{project_id} response.
type Incident struct {
	IssueNumber       int     `json:"issue_number,omitempty"`
	FiredAt           string  `json:"fired_at"`
	ResolvedAt        string  `json:"resolved_at,omitempty"`
	BreachDurationSec int64   `json:"breach_duration_sec,omitempty"`
	Ongoing           bool    `json:"ongoing"`
	BudgetConsumedPct float64 `json:"budget_consumed_pct"`
}

// IncidentsHandler serves GET /incidents/{project_id}.
func IncidentsHandler(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("project_id")
		alerts, err := store.ListAlerts(id)
		if err != nil {
			log.Printf("incidents %s: %v", id, err)
			http.Error(w, "list failed", http.StatusInternalServerError)
			return
		}
		out := make([]Incident, 0, len(alerts))
		for _, a := range alerts {
			inc := Incident{
				IssueNumber:       a.IssueNumber,
				FiredAt:           a.FiredAt.Format(time.RFC3339),
				BudgetConsumedPct: a.BudgetConsumedPct,
				Ongoing:           a.ResolvedAt.IsZero(),
			}
			if !a.ResolvedAt.IsZero() {
				inc.ResolvedAt = a.ResolvedAt.Format(time.RFC3339)
				inc.BreachDurationSec = int64(a.ResolvedAt.Sub(a.FiredAt).Seconds())
			}
			out = append(out, inc)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}
