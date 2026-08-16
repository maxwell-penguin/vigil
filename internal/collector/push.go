package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"vigil/internal/models"
	"vigil/internal/selfmetrics"
)

type Ingester interface {
	InsertEvent(models.Event) error
}

// pushEvent is the wire format for POST /ingest (project_id via ?project_id=).
type pushEvent struct {
	Timestamp  int64 `json:"timestamp"` // unix seconds; 0 = now
	LatencyMS  int64 `json:"latency_ms"`
	StatusCode int   `json:"status_code"`
	Error      bool  `json:"error,omitempty"` // explicit error flag, OR'd with status_code >= 400
}

func (p pushEvent) validate() error {
	if p.LatencyMS < 0 {
		return fmt.Errorf("latency_ms must be >= 0")
	}
	if p.StatusCode < 100 || p.StatusCode > 599 {
		return fmt.Errorf("status_code out of range")
	}
	return nil
}

// IngestHandler returns POST /ingest?project_id=X handler.
func IngestHandler(store Ingester) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		projectID := r.URL.Query().Get("project_id")
		if projectID == "" {
			http.Error(w, "project_id query param required", http.StatusBadRequest)
			return
		}

		var events []pushEvent
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&events); err != nil {
			http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(events) == 0 {
			http.Error(w, "empty array", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		for i, pe := range events {
			if err := pe.validate(); err != nil {
				http.Error(w, fmt.Sprintf("event[%d]: %v", i, err), http.StatusBadRequest)
				return
			}
			ts := now
			if pe.Timestamp > 0 {
				ts = time.Unix(pe.Timestamp, 0).UTC()
			}
			if err := store.InsertEvent(models.Event{
				ProjectID:  projectID,
				Timestamp:  ts,
				LatencyMS:  pe.LatencyMS,
				StatusCode: pe.StatusCode,
				Error:      pe.Error || pe.StatusCode >= 400,
			}); err != nil {
				log.Printf("ingest insert: %v", err)
				http.Error(w, "storage error", http.StatusInternalServerError)
				return
			}
		}
		selfmetrics.IngestEventsReceived.Add(int64(len(events)))
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"accepted":%d}`, len(events))
	}
}
