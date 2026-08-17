package collector

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vigil/internal/models"
)

// fakeIngester scripts one return value per InsertEvent call, in call order
// — the Nth call returns results[N], or nil once results is exhausted.
type fakeIngester struct {
	results []error
	calls   int
}

func (f *fakeIngester) InsertEvent(models.Event) error {
	var err error
	if f.calls < len(f.results) {
		err = f.results[f.calls]
	}
	f.calls++
	return err
}

type ingestResponse struct {
	Accepted int `json:"accepted"`
	Dropped  int `json:"dropped"`
}

func mkBody(t *testing.T, n int) []byte {
	t.Helper()
	events := make([]pushEvent, n)
	for i := range events {
		events[i] = pushEvent{LatencyMS: 50, StatusCode: 200}
	}
	b, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func doIngest(t *testing.T, store Ingester, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ingest?project_id=p", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	IngestHandler(store)(rec, req)
	return rec
}

// ponytail: one self-check for the fully-accepted path — every event lands,
// so the handler must report 202 with dropped=0, not the plain 202 it used
// to return before accepted/dropped counts existed.
func TestIngestHandlerAllAccepted(t *testing.T) {
	fake := &fakeIngester{}
	rec := doIngest(t, fake, mkBody(t, 3))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var got ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Accepted != 3 || got.Dropped != 0 {
		t.Fatalf("body = %+v, want accepted=3 dropped=0", got)
	}
}

// ponytail: one self-check for the partial-drop path — some events land,
// some don't, and the handler must report 200 (not 202) with counts that
// match exactly, since the batch was only partially accepted.
func TestIngestHandlerPartialDrop(t *testing.T) {
	fake := &fakeIngester{results: []error{nil, models.ErrQueueFull, nil, models.ErrQueueFull}}
	rec := doIngest(t, fake, mkBody(t, 4))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Accepted != 2 || got.Dropped != 2 {
		t.Fatalf("body = %+v, want accepted=2 dropped=2", got)
	}
}

// ponytail: one self-check for the fully-dropped path — the write queue is
// saturated for the whole batch, so the handler must report 503 with
// Retry-After, telling the caller to back off rather than silently losing
// every event under a 2xx status.
func TestIngestHandlerFullDrop(t *testing.T) {
	fake := &fakeIngester{results: []error{models.ErrQueueFull, models.ErrQueueFull, models.ErrQueueFull}}
	rec := doIngest(t, fake, mkBody(t, 3))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" {
		t.Fatalf("Retry-After header missing")
	}

	var got ingestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Accepted != 0 || got.Dropped != 3 {
		t.Fatalf("body = %+v, want accepted=0 dropped=3", got)
	}
}
