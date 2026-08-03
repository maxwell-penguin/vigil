package badge

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vigil/internal/models"
	"vigil/internal/storage"
)

// ponytail: one self-check per branch that actually varies the output —
// healthy (green, percentage) vs breaching (red, "BREACHING").
func TestHandlerColorsByBreachState(t *testing.T) {
	s, err := storage.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	slos := []models.SLO{{ProjectID: "p", TargetPct: 99, LatencyThresholdMS: 1000, WindowDays: 30}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /badge/{project_id}", Handler(s, slos))

	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		if err := s.InsertEvent(models.Event{
			ProjectID: "p", Timestamp: now.Add(-time.Duration(i) * time.Second),
			LatencyMS: 50, StatusCode: 200, Error: false,
		}); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/badge/p", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, colorGood) {
		t.Fatalf("expected healthy badge to use good color, got: %s", body)
	}
	if !strings.Contains(body, "%") || strings.Contains(body, "BREACHING") {
		t.Fatalf("expected percentage message on healthy badge, got: %s", body)
	}

	for i := 0; i < 30; i++ {
		if err := s.InsertEvent(models.Event{
			ProjectID: "p", Timestamp: now.Add(-time.Duration(i) * time.Second),
			LatencyMS: 50, StatusCode: 500, Error: true,
		}); err != nil {
			t.Fatal(err)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/badge/p", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body = rec.Body.String()
	if !strings.Contains(body, colorCritical) || !strings.Contains(body, "BREACHING") {
		t.Fatalf("expected breaching badge to be red with BREACHING text, got: %s", body)
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("expected Cache-Control: no-cache, got %q", rec.Header().Get("Cache-Control"))
	}
}
