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
	"vigil/internal/selfmetrics"
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

func TestSelfHandlerExposesMetrics(t *testing.T) {
	// Counters are process-global, so assert on the delta this test causes
	// rather than absolute values — robust to other tests/runs touching them.
	baseReceived := selfmetrics.IngestEventsReceived.Load()
	baseDropped := selfmetrics.IngestEventsDropped.Load()
	baseWebhook := selfmetrics.WebhookFailures.Load()
	baseGitHub := selfmetrics.GitHubIssueFailures.Load()
	baseChecks := selfmetrics.SLOChecksRun.Load()

	selfmetrics.IngestEventsReceived.Add(5)
	selfmetrics.IngestEventsDropped.Add(2)
	selfmetrics.WriteQueueDepth.Store(7)
	selfmetrics.WebhookFailures.Add(3)
	selfmetrics.GitHubIssueFailures.Add(1)
	selfmetrics.SLOChecksRun.Add(42)

	req := httptest.NewRequest(http.MethodGet, "/prometheus/vigil-internal", nil)
	rec := httptest.NewRecorder()
	SelfHandler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q", ct)
	}

	body := rec.Body.String()
	want := map[string]int64{
		"vigil_internal_ingest_received_total":  baseReceived + 5,
		"vigil_internal_ingest_dropped_total":   baseDropped + 2,
		"vigil_internal_write_queue_depth":      7,
		"vigil_internal_webhook_failures_total": baseWebhook + 3,
		"vigil_internal_github_failures_total":  baseGitHub + 1,
		"vigil_internal_slo_checks_total":       baseChecks + 42,
	}
	for name, wantVal := range want {
		if !strings.Contains(body, "# HELP "+name+" ") {
			t.Errorf("missing HELP line for %s\nbody:\n%s", name, body)
		}
		if !strings.Contains(body, "# TYPE "+name+" ") {
			t.Errorf("missing TYPE line for %s\nbody:\n%s", name, body)
		}

		re := regexp.MustCompile(`(?m)^` + name + ` (\S+)$`)
		match := re.FindStringSubmatch(body)
		if match == nil {
			t.Errorf("missing metric line for %s\nbody:\n%s", name, body)
			continue
		}
		got, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			t.Errorf("%s value %q is not an integer: %v", name, match[1], err)
			continue
		}
		if got != wantVal {
			t.Errorf("%s = %d, want %d", name, got, wantVal)
		}
	}
}
