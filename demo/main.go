// Standalone demo app: a toy HTTP service that simulates realistic traffic
// and reports every request straight to vigil's /ingest endpoint with a raw
// http.Post — no SDK dependency, to show the wire format is just JSON.
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

const projectID = "demo-app"

type event struct {
	Timestamp  int64 `json:"timestamp"`
	LatencyMS  int64 `json:"latency_ms"`
	StatusCode int   `json:"status_code"`
	Error      bool  `json:"error"`
}

func main() {
	vigilURL := os.Getenv("VIGIL_URL")
	if vigilURL == "" {
		vigilURL = "http://localhost:8080"
	}

	http.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		latency := time.Duration(50+rand.Intn(251)) * time.Millisecond // 50-300ms
		time.Sleep(latency)

		status := http.StatusOK
		if rand.Float64() < 0.01 { // 1% error rate
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)
		w.Write([]byte(`{"status":"ok"}`))

		go report(vigilURL, latency, status)
	})

	log.Printf("demo-app listening on :9090, reporting to %s", vigilURL)
	log.Fatal(http.ListenAndServe(":9090", nil))
}

func report(vigilURL string, latency time.Duration, status int) {
	body, err := json.Marshal([]event{{
		Timestamp:  time.Now().Unix(),
		LatencyMS:  latency.Milliseconds(),
		StatusCode: status,
		Error:      status >= 400,
	}})
	if err != nil {
		log.Printf("demo-app: marshal event: %v", err)
		return
	}

	url := vigilURL + "/ingest?project_id=" + projectID
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("demo-app: report event: %v", err)
		return
	}
	resp.Body.Close()
}
