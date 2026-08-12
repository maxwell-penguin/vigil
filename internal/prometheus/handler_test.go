package prometheus

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"vigil/internal/models"
	"vigil/internal/storage"
)

func TestHandlerExposesMetrics(t *testing.T) {
	s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	slos := []models.SLO{{ProjectID: "p", TargetPct: 99, LatencyThresholdMS: 1000, WindowDays: 30}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /prometheus/{project_id}", Handler(s, slos))

	now := time.Now().UTC()
	// InsertEvent only queues for the async writer; StartWriter isn't
	// running in this test, so seed directly via InsertEventsBatch instead.
	var events []models.Event
	for i := 0; i < 100; i++ {
		e := models.Event{
			ProjectID: "p", Timestamp: now.Add(-time.Duration(i) * time.Second),
			LatencyMS: int64(50 + i), StatusCode: 200, Error: false,
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
	if err := s.InsertAlert(models.Alert{ProjectID: "p", FiredAt: now.Add(-time.Hour), ResolvedAt: now.Add(-50 * time.Minute)}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/prometheus/p", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q", ct)
	}

	body := rec.Body.String()
	metrics := []string{
		"vigil_slo_compliance",
		"vigil_error_budget_remaining",
		"vigil_burn_rate_short",
		"vigil_burn_rate_long",
		"vigil_slo_breaching",
		"vigil_incidents_total",
		"vigil_p99_latency_ms",
	}
	for _, m := range metrics {
		if !strings.Contains(body, "# HELP "+m+" ") {
			t.Errorf("missing HELP line for %s\nbody:\n%s", m, body)
		}
		if !strings.Contains(body, "# TYPE "+m+" ") {
			t.Errorf("missing TYPE line for %s\nbody:\n%s", m, body)
		}

		re := regexp.MustCompile(`(?m)^` + m + `\{project_id="p"\} (\S+)$`)
		match := re.FindStringSubmatch(body)
		if match == nil {
			t.Errorf("missing metric line for %s\nbody:\n%s", m, body)
			continue
		}
		if _, err := strconv.ParseFloat(match[1], 64); err != nil {
			t.Errorf("%s value %q is not numeric: %v", m, match[1], err)
		}
	}

	if !strings.Contains(body, `vigil_incidents_total{project_id="p"} 1`) {
		t.Errorf("expected 1 recorded incident, got:\n%s", body)
	}
}

func TestHandlerUnknownProjectReturns404(t *testing.T) {
	s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /prometheus/{project_id}", Handler(s, nil))

	req := httptest.NewRequest(http.MethodGet, "/prometheus/nope", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
