// Package prometheus serves a per-project Prometheus scrape endpoint,
// written directly in exposition format — no client library needed for
// six static gauges/counters.
package prometheus

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"vigil/internal/models"
	"vigil/internal/slo"
	"vigil/internal/storage"
)

// Handler serves GET /prometheus/{project_id}.
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
			log.Printf("prometheus %s: compute: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}

		alerts, err := store.ListAlerts(id)
		if err != nil {
			log.Printf("prometheus %s: list alerts: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}

		p99, err := store.LatencyPercentile(id, now.Add(-slo.LongWindow), now, 99)
		if err != nil {
			log.Printf("prometheus %s: p99: %v", id, err)
			http.Error(w, "compute failed", http.StatusInternalServerError)
			return
		}

		breaching := 0
		if st.IsBreaching {
			breaching = 1
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP vigil_slo_compliance Current SLO compliance percentage (0-100)\n")
		fmt.Fprintf(w, "# TYPE vigil_slo_compliance gauge\n")
		fmt.Fprintf(w, "vigil_slo_compliance{project_id=%q} %.2f\n\n", id, st.SLOPct)

		fmt.Fprintf(w, "# HELP vigil_error_budget_remaining Error budget remaining percentage (0-100)\n")
		fmt.Fprintf(w, "# TYPE vigil_error_budget_remaining gauge\n")
		fmt.Fprintf(w, "vigil_error_budget_remaining{project_id=%q} %.1f\n\n", id, st.BudgetRemainingPct)

		fmt.Fprintf(w, "# HELP vigil_burn_rate_short Short window (5m) burn rate\n")
		fmt.Fprintf(w, "# TYPE vigil_burn_rate_short gauge\n")
		fmt.Fprintf(w, "vigil_burn_rate_short{project_id=%q} %.2f\n\n", id, st.ShortBurnRate)

		fmt.Fprintf(w, "# HELP vigil_burn_rate_long Long window (1h) burn rate\n")
		fmt.Fprintf(w, "# TYPE vigil_burn_rate_long gauge\n")
		fmt.Fprintf(w, "vigil_burn_rate_long{project_id=%q} %.2f\n\n", id, st.LongBurnRate)

		fmt.Fprintf(w, "# HELP vigil_slo_breaching Whether the SLO is currently breaching (0 or 1)\n")
		fmt.Fprintf(w, "# TYPE vigil_slo_breaching gauge\n")
		fmt.Fprintf(w, "vigil_slo_breaching{project_id=%q} %d\n\n", id, breaching)

		fmt.Fprintf(w, "# HELP vigil_incidents_total Total number of incidents recorded\n")
		fmt.Fprintf(w, "# TYPE vigil_incidents_total counter\n")
		fmt.Fprintf(w, "vigil_incidents_total{project_id=%q} %d\n\n", id, len(alerts))

		fmt.Fprintf(w, "# HELP vigil_p99_latency_ms Current p99 latency in milliseconds\n")
		fmt.Fprintf(w, "# TYPE vigil_p99_latency_ms gauge\n")
		fmt.Fprintf(w, "vigil_p99_latency_ms{project_id=%q} %.1f\n", id, float64(p99))
	}
}
