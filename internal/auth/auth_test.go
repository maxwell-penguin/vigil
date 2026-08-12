package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestMiddlewareNoOpWhenKeyEmpty(t *testing.T) {
	h := Middleware("")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no key configured means pass through)", rec.Code)
	}
}

func TestMiddlewareAcceptsCorrectToken(t *testing.T) {
	h := Middleware("secret")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestMiddlewareRejectsWrongToken(t *testing.T) {
	h := Middleware("secret")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"error":"unauthorized"`) {
		t.Errorf("body = %q, want it to contain the unauthorized error", body)
	}
}

func TestMiddlewareRejectsMissingHeader(t *testing.T) {
	h := Middleware("secret")(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// TestPublicEndpointsBypassAuth mirrors how main.go actually wires routes:
// badge/status/healthz are registered directly, never passed through
// Middleware, so they must stay reachable even with a key configured.
func TestPublicEndpointsBypassAuthWhenKeyConfigured(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /slo/{project_id}", Middleware("secret")(okHandler()))
	mux.Handle("GET /badge/{project_id}", okHandler())
	mux.Handle("GET /status/{project_id}", okHandler())
	mux.Handle("GET /healthz", okHandler())

	for _, path := range []string{"/badge/demo", "/status/demo", "/healthz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (public endpoint should bypass auth)", path, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/slo/demo", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("/slo/demo: status = %d, want 401 (protected endpoint)", rec.Code)
	}
}
