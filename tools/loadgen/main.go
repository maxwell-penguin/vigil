// Command loadgen fires concurrent batches of synthetic events at a running
// Vigil server's POST /ingest endpoint and reports latency/throughput plus
// server-side queue depth and drop counts pulled from the internal
// Prometheus endpoint.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type pushEvent struct {
	Timestamp  int64 `json:"timestamp,omitempty"`
	LatencyMS  int64 `json:"latency_ms"`
	StatusCode int   `json:"status_code"`
}

type ingestResponse struct {
	Accepted int `json:"accepted"`
	Dropped  int `json:"dropped"`
}

type result struct {
	latency  time.Duration
	status   int // 0 = transport-level error, no HTTP response
	accepted int
	dropped  int
}

func main() {
	url := flag.String("url", "http://localhost:8080", "target base URL")
	project := flag.String("project", "loadtest", "project_id to ingest into")
	apiKey := flag.String("api-key", "", "value for Authorization: Bearer header (empty = no header)")
	workers := flag.Int("workers", 50, "concurrent goroutines")
	duration := flag.Duration("duration", 10*time.Second, "how long to run")
	batchSize := flag.Int("batch-size", 10, "events per POST")
	flag.Parse()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        500,
			MaxIdleConnsPerHost: 200,
			IdleConnTimeout:     90 * time.Second,
		},
	}
	endpoint := strings.TrimRight(*url, "/") + "/ingest?project_id=" + *project

	before, err := fetchInternalMetrics(client, *url)
	if err != nil {
		log.Fatalf("baseline metrics fetch: %v", err)
	}

	results := make([][]result, *workers)
	var wg sync.WaitGroup
	deadline := time.Now().Add(*duration)
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(w)))
			var local []result
			for time.Now().Before(deadline) {
				local = append(local, doRequest(client, endpoint, *apiKey, *batchSize, rng))
			}
			results[w] = local
		}(w)
	}
	wg.Wait()

	after, err := fetchInternalMetrics(client, *url)
	if err != nil {
		log.Fatalf("final metrics fetch: %v", err)
	}

	printSummary(results, before, after)
}

func doRequest(client *http.Client, endpoint, apiKey string, batchSize int, rng *rand.Rand) result {
	events := make([]pushEvent, batchSize)
	for i := range events {
		status := 200
		if rng.Intn(10) == 0 {
			status = 500
		}
		events[i] = pushEvent{
			LatencyMS:  50 + rng.Int63n(251),
			StatusCode: status,
		}
	}
	body, err := json.Marshal(events)
	if err != nil {
		log.Fatalf("marshal batch: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		log.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return result{latency: latency}
	}
	defer resp.Body.Close()

	// Always drain the body fully so the connection can be reused, even
	// when the status/shape doesn't match ingestResponse (e.g. 401 errors).
	data, _ := io.ReadAll(resp.Body)
	var ir ingestResponse
	_ = json.Unmarshal(data, &ir) // best-effort; error bodies aren't this shape

	return result{
		latency:  latency,
		status:   resp.StatusCode,
		accepted: ir.Accepted,
		dropped:  ir.Dropped,
	}
}

type internalMetrics struct {
	writeQueueDepth int64
	ingestDropped   int64
}

func fetchInternalMetrics(client *http.Client, baseURL string) (internalMetrics, error) {
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/prometheus/vigil-internal")
	if err != nil {
		return internalMetrics{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return internalMetrics{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var m internalMetrics
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "vigil_internal_write_queue_depth":
			m.writeQueueDepth, _ = strconv.ParseInt(fields[1], 10, 64)
		case "vigil_internal_ingest_dropped_total":
			m.ingestDropped, _ = strconv.ParseInt(fields[1], 10, 64)
		}
	}
	return m, sc.Err()
}

func printSummary(results [][]result, before, after internalMetrics) {
	var all []result
	for _, local := range results {
		all = append(all, local...)
	}

	byStatus := make(map[int]int)
	var totalAccepted, totalDropped int
	latencies := make([]time.Duration, 0, len(all))
	for _, r := range all {
		byStatus[r.status]++
		totalAccepted += r.accepted
		totalDropped += r.dropped
		latencies = append(latencies, r.latency)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	fmt.Printf("\n=== loadgen summary ===\n")
	fmt.Printf("total requests:    %d\n", len(all))
	fmt.Printf("status breakdown:\n")
	statuses := make([]int, 0, len(byStatus))
	for s := range byStatus {
		statuses = append(statuses, s)
	}
	sort.Ints(statuses)
	for _, s := range statuses {
		label := strconv.Itoa(s)
		if s == 0 {
			label = "transport error"
		}
		fmt.Printf("  %-16s %d\n", label, byStatus[s])
	}
	fmt.Printf("accepted (bodies): %d\n", totalAccepted)
	fmt.Printf("dropped (bodies):  %d\n", totalDropped)
	fmt.Printf("server queue depth: %d -> %d\n", before.writeQueueDepth, after.writeQueueDepth)
	fmt.Printf("server dropped delta: %d\n", after.ingestDropped-before.ingestDropped)

	if len(latencies) > 0 {
		fmt.Printf("client latency (ms): p50=%.1f p95=%.1f p99=%.1f\n",
			percentile(latencies, 50), percentile(latencies, 95), percentile(latencies, 99))
	}
}

func percentile(sorted []time.Duration, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := len(sorted) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return float64(sorted[idx]) / float64(time.Millisecond)
}
