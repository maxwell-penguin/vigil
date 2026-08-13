package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func doRequest(h http.Handler, projectID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/ingest?project_id="+projectID, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMiddlewareAllowsWithinBurst(t *testing.T) {
	h := Middleware(okHandler())
	projectID := "burst-ok"

	for i := 0; i < burst; i++ {
		rec := doRequest(h, projectID)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i, rec.Code)
		}
	}
}

func TestMiddlewareRejectsBeyondBurst(t *testing.T) {
	h := Middleware(okHandler())
	projectID := "burst-exceeded"

	for i := 0; i < burst; i++ {
		doRequest(h, projectID)
	}

	rec := doRequest(h, projectID)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	want := `{"error":"rate limit exceeded","project_id":"burst-exceeded"}`
	if body := rec.Body.String(); body != want+"\n" {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestMiddlewareLimitersAreIndependentPerProject(t *testing.T) {
	h := Middleware(okHandler())

	for i := 0; i < burst; i++ {
		doRequest(h, "project-a")
	}
	if rec := doRequest(h, "project-a"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("project-a: status = %d, want 429 (should be exhausted)", rec.Code)
	}

	if rec := doRequest(h, "project-b"); rec.Code != http.StatusOK {
		t.Fatalf("project-b: status = %d, want 200 (independent limiter)", rec.Code)
	}
}
