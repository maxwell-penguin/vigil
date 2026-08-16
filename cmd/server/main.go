package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"vigil/internal/auth"
	"vigil/internal/badge"
	"vigil/internal/collector"
	"vigil/internal/demo"
	"vigil/internal/github"
	"vigil/internal/models"
	"vigil/internal/notify"
	"vigil/internal/prometheus"
	"vigil/internal/ratelimit"
	"vigil/internal/slo"
	"vigil/internal/status"
	"vigil/internal/storage"
)

func main() {
	cfgPath := flag.String("config", "vigil.yaml", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if dbPath := os.Getenv("VIGIL_DB_PATH"); dbPath != "" {
		cfg.DBPath = dbPath
	}

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer store.Close()

	writerCtx, cancelWriter := context.WithCancel(context.Background())
	defer cancelWriter()
	store.StartWriter(writerCtx)

	var notifier slo.Notifier
	if cfg.GitHubToken != "" && cfg.GitHubRepo != "" {
		ghClient := github.NewClient(cfg.GitHubToken, cfg.GitHubRepo)
		notifier = github.NewIncidentManager(ghClient, store)
	} else {
		log.Printf("github_token/github_repo not set: SLO breaches will only be logged")
	}

	var webhookNotifier *notify.WebhookNotifier
	if cfg.WebhookURL != "" {
		webhookNotifier = notify.NewWebhookNotifier(cfg.WebhookURL, cfg.WebhookType)
	}

	stop := make(chan struct{})

	go collector.NewProber(cfg.Probes, store, 30*time.Second).Run(stop)
	go store.RunDownsampleLoop(1*time.Hour, stop)
	go slo.NewChecker(store, cfg.SLOs, 60*time.Second, notifier, webhookNotifier).Run(stop)

	// Wraps a handler with CORS then auth (auth runs first, so an
	// unauthorized request never gets a CORS header). apiKey == "" makes
	// authMW a no-op, so this is harmless when auth is disabled.
	authMW := auth.Middleware(cfg.APIKey)
	protect := func(h http.HandlerFunc) http.Handler {
		return authMW(withCORS(h))
	}

	mux := http.NewServeMux()
	mux.Handle("POST /ingest", authMW(ratelimit.Middleware(collector.IngestHandler(store))))
	mux.Handle("GET /slo/{project_id}", protect(slo.StatusHandler(store, cfg.SLOs)))
	mux.Handle("GET /incidents/{project_id}", protect(slo.IncidentsHandler(store)))
	mux.Handle("GET /metrics/{project_id}", protect(metricsHandler(store)))
	mux.Handle("GET /prometheus/{project_id}", protect(prometheus.Handler(store, cfg.SLOs)))
	// Always public, same as /healthz — internal observability, not project data.
	mux.HandleFunc("GET /prometheus/vigil-internal", prometheus.SelfHandler())
	mux.Handle("GET /demo", protect(demo.Handler(store)))
	// Always public, even with an API key configured — never passed
	// through authMW.
	mux.HandleFunc("GET /badge/{project_id}", withCORS(badge.Handler(store, cfg.SLOs)))
	mux.HandleFunc("GET /status/{project_id}", status.Handler(store, cfg.SLOs))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		close(stop)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx) // drain in-flight /ingest requests first...
		cancelWriter()        // ...then flush whatever they queued
	}()

	log.Printf("vigil listening on %s (db=%s, %d probes, %d slos)",
		srv.Addr, cfg.DBPath, len(cfg.Probes), len(cfg.SLOs))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

// metricsHandler serves GET /metrics/{project_id}: hourly p50/p95/p99 latency
// buckets over the last 24h, for the dashboard's latency chart.
func metricsHandler(store *storage.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("project_id")
		now := time.Now()
		buckets, err := store.LatencyPercentilesBucketed(id, time.Hour, now.Add(-24*time.Hour), now)
		if err != nil {
			log.Printf("metrics %s: %v", id, err)
			http.Error(w, "query failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(buckets)
	}
}

// withCORS allows the dashboard (served from a different origin in dev) to
// read these GET-only endpoints. Fine for public health/status data.
func withCORS(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h(w, r)
	}
}

func loadConfig(path string) (models.Config, error) {
	var cfg models.Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "vigil.db"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	return cfg, nil
}
