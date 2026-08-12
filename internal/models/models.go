package models

import "time"

// Event is a single observation for a project at a point in time.
type Event struct {
	ProjectID  string    `json:"project_id,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	LatencyMS  int64     `json:"latency_ms"`
	StatusCode int       `json:"status_code"`
	Error      bool      `json:"error"`
}

// Metric is one aggregated bucket returned by GetMetrics.
type Metric struct {
	ProjectID   string    `json:"project_id"`
	BucketStart time.Time `json:"bucket_start"`
	Resolution  string    `json:"resolution"`
	GoodEvents  int64     `json:"good_events"`
	TotalEvents int64     `json:"total_events"`
}

// ProbeTarget is one external endpoint the prober should hit.
type ProbeTarget struct {
	ProjectID      string `yaml:"project_id"`
	URL            string `yaml:"url"`
	ExpectedStatus int    `yaml:"expected_status"`
}

// LatencyBucket is one point in a p50/p95/p99 latency time series.
type LatencyBucket struct {
	BucketStart time.Time `json:"bucket_start"`
	P50MS       int64     `json:"p50_ms"`
	P95MS       int64     `json:"p95_ms"`
	P99MS       int64     `json:"p99_ms"`
}

// Alert is one recorded SLO breach.
type Alert struct {
	ProjectID         string    `json:"project_id"`
	FiredAt           time.Time `json:"fired_at"`
	IssueNumber       int       `json:"issue_number,omitempty"`
	ResolvedAt        time.Time `json:"resolved_at,omitempty"`
	BudgetConsumedPct float64   `json:"budget_consumed_pct"`
	// SLOPct/TargetPct/burn rates are a snapshot of Status at the moment the
	// alert fired, so the incident detail view can show what was true then
	// even after live SLO% has since recovered.
	SLOPct        float64 `json:"slo_pct,omitempty"`
	TargetPct     float64 `json:"target_pct,omitempty"`
	ShortBurnRate float64 `json:"short_burn_rate,omitempty"`
	LongBurnRate  float64 `json:"long_burn_rate,omitempty"`
}

// SLO is a service level objective definition for a project.
// A "good event" = status code < 400 AND latency_ms <= LatencyThresholdMS.
type SLO struct {
	ProjectID          string  `yaml:"project_id"`
	TargetPct          float64 `yaml:"target_pct"`
	LatencyThresholdMS int64   `yaml:"latency_threshold_ms"`
	WindowDays         int     `yaml:"window_days"`
}

// Config is the on-disk vigil.yaml.
type Config struct {
	DBPath      string        `yaml:"db_path"`
	Port        int           `yaml:"port"`
	Probes      []ProbeTarget `yaml:"probes"`
	SLOs        []SLO         `yaml:"slos"`
	GitHubToken string        `yaml:"github_token"`
	GitHubRepo  string        `yaml:"github_repo"` // "owner/repo"
	WebhookURL  string        `yaml:"webhook_url"`
	WebhookType string        `yaml:"webhook_type"` // "slack" or "discord"
	APIKey      string        `yaml:"api_key"`
}
