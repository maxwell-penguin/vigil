package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vigil/internal/models"
	"vigil/internal/slo"
)

type fakeStore struct {
	alert models.Alert
	ok    bool
}

func (f *fakeStore) LatestAlert(string) (models.Alert, bool, error) { return f.alert, f.ok, nil }

// ponytail: one self-check for the dedup branch — the whole point of task 3.
// Case A: previous issue still open on GitHub -> reuse its number, no POST.
// Case B: previous issue closed -> a new issue is created.
func TestNotifyBreachDedup(t *testing.T) {
	var (
		getCalls, postCalls int
		issueState          = "open"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			getCalls++
			json.NewEncoder(w).Encode(Issue{Number: 42, State: issueState})
		case r.Method == http.MethodPost:
			postCalls++
			json.NewEncoder(w).Encode(Issue{Number: 99, State: "open"})
		}
	}))
	defer srv.Close()

	client := NewClient("tok", "owner/repo")
	client.HTTPClient = srv.Client()
	// redirect apiBase-shaped calls at the test server via a wrapping RoundTripper
	client.HTTPClient.Transport = rewriteHost{srv.URL}

	store := &fakeStore{alert: models.Alert{ProjectID: "p", IssueNumber: 42}, ok: true}
	mgr := NewIncidentManager(client, store)
	s := models.SLO{ProjectID: "p", TargetPct: 99}
	st := slo.Status{IsBreaching: true}

	num, err := mgr.NotifyBreach(s, st, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if num != 42 || postCalls != 0 || getCalls != 1 {
		t.Fatalf("want reuse of open issue 42, got num=%d get=%d post=%d", num, getCalls, postCalls)
	}

	issueState = "closed"
	num, err = mgr.NotifyBreach(s, st, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if num != 99 || postCalls != 1 {
		t.Fatalf("want new issue 99 after closed, got num=%d post=%d", num, postCalls)
	}
}

// rewriteHost sends every request to the test server regardless of the
// api.github.com host baked into Client, without changing Client's production code.
type rewriteHost struct{ base string }

func (rw rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := http.NewRequest(req.Method, rw.base+req.URL.Path, req.Body)
	if err != nil {
		return nil, err
	}
	target.Header = req.Header
	return http.DefaultTransport.RoundTrip(target)
}
