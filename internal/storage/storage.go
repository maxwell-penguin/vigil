package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"vigil/internal/models"
)

const (
	eventQueueSize     = 1000
	eventBatchSize     = 100
	eventFlushInterval = 500 * time.Millisecond
)

const schema = `
CREATE TABLE IF NOT EXISTS raw_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT    NOT NULL,
    timestamp   INTEGER NOT NULL,
    latency_ms  INTEGER NOT NULL,
    status_code INTEGER NOT NULL,
    error       INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_raw_ts       ON raw_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_raw_proj_ts  ON raw_events(project_id, timestamp);

CREATE TABLE IF NOT EXISTS aggregated_metrics (
    project_id   TEXT    NOT NULL,
    bucket_start INTEGER NOT NULL,
    resolution   TEXT    NOT NULL,
    good_events  INTEGER NOT NULL,
    total_events INTEGER NOT NULL,
    PRIMARY KEY (project_id, bucket_start, resolution)
);
CREATE INDEX IF NOT EXISTS idx_agg_proj_ts ON aggregated_metrics(project_id, bucket_start);

CREATE TABLE IF NOT EXISTS slo_alerts (
    project_id          TEXT    NOT NULL,
    fired_at            INTEGER NOT NULL,
    issue_number        INTEGER NOT NULL DEFAULT 0,
    resolved_at         INTEGER NOT NULL DEFAULT 0,
    budget_consumed_pct REAL    NOT NULL DEFAULT 0,
    slo_pct             REAL    NOT NULL DEFAULT 0,
    target_pct          REAL    NOT NULL DEFAULT 0,
    short_burn_rate     REAL    NOT NULL DEFAULT 0,
    long_burn_rate      REAL    NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_slo_alerts_proj_ts ON slo_alerts(project_id, fired_at);
`

// alertSnapshotColumns are the columns added to slo_alerts after its initial
// release. ADD COLUMN has no IF NOT EXISTS in SQLite, so existing DBs need
// this to run once and be skipped afterward.
var alertSnapshotColumns = []string{"slo_pct", "target_pct", "short_burn_rate", "long_burn_rate"}

func migrateAlertColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(slo_alerts)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	have := make(map[string]bool)
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		have[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, col := range alertSnapshotColumns {
		if have[col] {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE slo_alerts ADD COLUMN ` + col + ` REAL NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add column %s: %w", col, err)
		}
	}
	return nil
}

type Store struct {
	db         *sql.DB
	eventQueue chan []models.Event
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// PRAGMAs set via Exec only bind to whichever connection runs them, not
	// to every connection database/sql may later open from the pool — pin
	// the pool to one connection so synchronous/busy_timeout reliably apply
	// on every query. Writes are already serialized through the event
	// queue's single writer goroutine, so this costs no real write
	// concurrency; it just keeps other queries off separate, unpragma'd
	// connections. journal_mode=WAL is persisted in the DB file itself, so
	// it doesn't strictly need this, but setting it here keeps all three
	// PRAGMAs in one place.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA synchronous=NORMAL;
		PRAGMA busy_timeout=5000;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrateAlertColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &Store{db: db, eventQueue: make(chan []models.Event, eventQueueSize)}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// InsertEvent enqueues a single event for the writer goroutine started by
// StartWriter to batch and flush. It never blocks: if the queue is full
// (StartWriter isn't running, or the DB can't keep up), the event is
// dropped and logged rather than stalling the caller's request.
func (s *Store) InsertEvent(e models.Event) error {
	select {
	case s.eventQueue <- []models.Event{e}:
	default:
		log.Printf("event queue full: dropping event for project %s", e.ProjectID)
	}
	return nil
}

// StartWriter launches the goroutine that drains eventQueue and flushes it
// to SQLite via InsertEventsBatch — batching up to eventBatchSize events or
// waiting up to eventFlushInterval, whichever comes first. It returns
// immediately; the goroutine exits once ctx is cancelled, flushing whatever
// remains queued.
func (s *Store) StartWriter(ctx context.Context) {
	go func() {
		batch := make([]models.Event, 0, eventBatchSize)
		ticker := time.NewTicker(eventFlushInterval)
		defer ticker.Stop()

		flush := func() {
			if len(batch) == 0 {
				return
			}
			if err := s.InsertEventsBatch(batch); err != nil {
				log.Printf("event writer: flush %d events: %v", len(batch), err)
			}
			batch = batch[:0]
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case events := <-s.eventQueue:
				batch = append(batch, events...)
				if len(batch) >= eventBatchSize {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}()
}

// Downsample rolls up raw_events older than 24h into 1m buckets,
// then rolls up 1m buckets older than 7d into 1h buckets, deleting the source rows.
func (s *Store) Downsample(now time.Time) error {
	if err := s.rollupRaw(now.Add(-24*time.Hour).Unix(), 60, "1m"); err != nil {
		return fmt.Errorf("raw->1m: %w", err)
	}
	if err := s.rollupAgg(now.Add(-7*24*time.Hour).Unix(), 3600, "1m", "1h"); err != nil {
		return fmt.Errorf("1m->1h: %w", err)
	}
	return nil
}

func (s *Store) rollupRaw(cutoff int64, bucketSec int64, resolution string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO aggregated_metrics (project_id, bucket_start, resolution, good_events, total_events)
		SELECT project_id,
		       (timestamp / ?) * ?      AS bucket_start,
		       ?                        AS resolution,
		       SUM(CASE WHEN error = 0 THEN 1 ELSE 0 END),
		       COUNT(*)
		FROM raw_events
		WHERE timestamp < ?
		GROUP BY project_id, bucket_start
		ON CONFLICT(project_id, bucket_start, resolution) DO UPDATE SET
		    good_events  = good_events  + excluded.good_events,
		    total_events = total_events + excluded.total_events
	`, bucketSec, bucketSec, resolution, cutoff)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM raw_events WHERE timestamp < ?`, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) rollupAgg(cutoff int64, bucketSec int64, from, to string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO aggregated_metrics (project_id, bucket_start, resolution, good_events, total_events)
		SELECT project_id,
		       (bucket_start / ?) * ?   AS new_bucket,
		       ?                        AS resolution,
		       SUM(good_events),
		       SUM(total_events)
		FROM aggregated_metrics
		WHERE resolution = ? AND bucket_start < ?
		GROUP BY project_id, new_bucket
		ON CONFLICT(project_id, bucket_start, resolution) DO UPDATE SET
		    good_events  = good_events  + excluded.good_events,
		    total_events = total_events + excluded.total_events
	`, bucketSec, bucketSec, to, from, cutoff)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(
		`DELETE FROM aggregated_metrics WHERE resolution = ? AND bucket_start < ?`,
		from, cutoff,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// GetMetrics returns aggregated buckets overlapping [from, to] for a project,
// plus a synthetic bucket per project derived from any still-raw events in range.
func (s *Store) GetMetrics(projectID string, from, to time.Time) ([]models.Metric, error) {
	rows, err := s.db.Query(`
		SELECT project_id, bucket_start, resolution, good_events, total_events
		FROM aggregated_metrics
		WHERE project_id = ? AND bucket_start >= ? AND bucket_start <= ?
		ORDER BY bucket_start ASC
	`, projectID, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	var out []models.Metric
	for rows.Next() {
		var m models.Metric
		var ts int64
		if err := rows.Scan(&m.ProjectID, &ts, &m.Resolution, &m.GoodEvents, &m.TotalEvents); err != nil {
			return nil, err
		}
		m.BucketStart = time.Unix(ts, 0).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountRawInWindow returns (good, total) over raw_events for a project in [from, to].
// "good" = error = 0 AND latency_ms <= latencyThresholdMS.
func (s *Store) CountRawInWindow(projectID string, from, to time.Time, latencyThresholdMS int64) (int64, int64, error) {
	var good, total int64
	err := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN error = 0 AND latency_ms <= ? THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM raw_events
		WHERE project_id = ? AND timestamp >= ? AND timestamp <= ?
	`, latencyThresholdMS, projectID, from.Unix(), to.Unix()).Scan(&good, &total)
	if err != nil {
		return 0, 0, fmt.Errorf("count raw: %w", err)
	}
	return good, total, nil
}

func (s *Store) InsertAlert(a models.Alert) error {
	var resolved int64
	if !a.ResolvedAt.IsZero() {
		resolved = a.ResolvedAt.Unix()
	}
	_, err := s.db.Exec(
		`INSERT INTO slo_alerts (project_id, fired_at, issue_number, resolved_at, budget_consumed_pct, slo_pct, target_pct, short_burn_rate, long_burn_rate)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ProjectID, a.FiredAt.Unix(), a.IssueNumber, resolved, a.BudgetConsumedPct,
		a.SLOPct, a.TargetPct, a.ShortBurnRate, a.LongBurnRate,
	)
	if err != nil {
		return fmt.Errorf("insert alert: %w", err)
	}
	return nil
}

// LatestAlert returns the most recent alert row for a project.
// The bool is false if the project has never alerted.
func (s *Store) LatestAlert(projectID string) (models.Alert, bool, error) {
	var (
		a              models.Alert
		firedAt, resAt int64
	)
	err := s.db.QueryRow(`
		SELECT project_id, fired_at, issue_number, resolved_at, budget_consumed_pct, slo_pct, target_pct, short_burn_rate, long_burn_rate
		FROM slo_alerts
		WHERE project_id = ?
		ORDER BY fired_at DESC
		LIMIT 1
	`, projectID).Scan(&a.ProjectID, &firedAt, &a.IssueNumber, &resAt, &a.BudgetConsumedPct,
		&a.SLOPct, &a.TargetPct, &a.ShortBurnRate, &a.LongBurnRate)
	if err == sql.ErrNoRows {
		return models.Alert{}, false, nil
	}
	if err != nil {
		return models.Alert{}, false, fmt.Errorf("latest alert: %w", err)
	}
	a.FiredAt = time.Unix(firedAt, 0).UTC()
	if resAt > 0 {
		a.ResolvedAt = time.Unix(resAt, 0).UTC()
	}
	return a, true, nil
}

// MarkAlertResolved stamps resolved_at on an existing alert row.
// Silently no-ops if it's already resolved.
func (s *Store) MarkAlertResolved(projectID string, firedAt, resolvedAt time.Time) error {
	_, err := s.db.Exec(
		`UPDATE slo_alerts SET resolved_at = ?
		 WHERE project_id = ? AND fired_at = ? AND resolved_at = 0`,
		resolvedAt.Unix(), projectID, firedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("mark resolved: %w", err)
	}
	return nil
}

// ListAlerts returns all alerts for a project, newest first.
func (s *Store) ListAlerts(projectID string) ([]models.Alert, error) {
	rows, err := s.db.Query(`
		SELECT project_id, fired_at, issue_number, resolved_at, budget_consumed_pct, slo_pct, target_pct, short_burn_rate, long_burn_rate
		FROM slo_alerts
		WHERE project_id = ?
		ORDER BY fired_at DESC
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()
	var out []models.Alert
	for rows.Next() {
		var (
			a              models.Alert
			firedAt, resAt int64
		)
		if err := rows.Scan(&a.ProjectID, &firedAt, &a.IssueNumber, &resAt, &a.BudgetConsumedPct,
			&a.SLOPct, &a.TargetPct, &a.ShortBurnRate, &a.LongBurnRate); err != nil {
			return nil, err
		}
		a.FiredAt = time.Unix(firedAt, 0).UTC()
		if resAt > 0 {
			a.ResolvedAt = time.Unix(resAt, 0).UTC()
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FetchRecentRawEvents returns the most recent n raw events for a project.
func (s *Store) FetchRecentRawEvents(projectID string, n int) ([]models.Event, error) {
	if n <= 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT project_id, timestamp, latency_ms, status_code, error
		FROM raw_events
		WHERE project_id = ?
		ORDER BY timestamp DESC, id DESC
		LIMIT ?
	`, projectID, n)
	if err != nil {
		return nil, fmt.Errorf("fetch raw: %w", err)
	}
	defer rows.Close()
	var out []models.Event
	for rows.Next() {
		var (
			e      models.Event
			ts     int64
			errBit int
		)
		if err := rows.Scan(&e.ProjectID, &ts, &e.LatencyMS, &e.StatusCode, &errBit); err != nil {
			return nil, err
		}
		e.Timestamp = time.Unix(ts, 0).UTC()
		e.Error = errBit == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// LatencyPercentile returns the p-th percentile (nearest-rank) latency in ms
// for a project's raw events in [from, to]. Only raw_events carries latency,
// so the window must fall within retention (Downsample keeps raw < 24h).
func (s *Store) LatencyPercentile(projectID string, from, to time.Time, p float64) (int64, error) {
	rows, err := s.db.Query(`
		SELECT latency_ms FROM raw_events
		WHERE project_id = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY latency_ms ASC
	`, projectID, from.Unix(), to.Unix())
	if err != nil {
		return 0, fmt.Errorf("latency percentile: %w", err)
	}
	defer rows.Close()

	var vals []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return 0, err
		}
		vals = append(vals, v)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return percentile(vals, p), nil
}

// LatencyPercentilesBucketed returns p50/p95/p99 per fixed-size time bucket
// over [from, to], for charting. One query fetches the whole range; latencies
// are grouped by bucket in Go and sorted per bucket to rank them.
func (s *Store) LatencyPercentilesBucketed(projectID string, bucketSize time.Duration, from, to time.Time) ([]models.LatencyBucket, error) {
	rows, err := s.db.Query(`
		SELECT timestamp, latency_ms FROM raw_events
		WHERE project_id = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp ASC
	`, projectID, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("latency buckets: %w", err)
	}
	defer rows.Close()

	bucketSec := int64(bucketSize.Seconds())
	grouped := make(map[int64][]int64)
	var order []int64
	for rows.Next() {
		var ts, lat int64
		if err := rows.Scan(&ts, &lat); err != nil {
			return nil, err
		}
		b := (ts / bucketSec) * bucketSec
		if _, seen := grouped[b]; !seen {
			order = append(order, b)
		}
		grouped[b] = append(grouped[b], lat)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.LatencyBucket, 0, len(order))
	for _, b := range order {
		vals := grouped[b]
		sort.Slice(vals, func(i, j int) bool { return vals[i] < vals[j] })
		out = append(out, models.LatencyBucket{
			BucketStart: time.Unix(b, 0).UTC(),
			P50MS:       percentile(vals, 50),
			P95MS:       percentile(vals, 95),
			P99MS:       percentile(vals, 99),
		})
	}
	return out, nil
}

// percentile uses nearest-rank on an already-sorted-ascending slice.
func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// DeleteProjectData removes every row for a project across all three
// tables. Used to make re-seeding (e.g. GET /demo) idempotent.
func (s *Store) DeleteProjectData(projectID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"raw_events", "aggregated_metrics", "slo_alerts"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE project_id = ?`, projectID); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	return tx.Commit()
}

// InsertEventsBatch inserts many raw events in a single transaction.
// Used by bulk producers (e.g. demo data seeding) instead of calling
// InsertEvent in a loop, which would commit once per row.
func (s *Store) InsertEventsBatch(events []models.Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT INTO raw_events (project_id, timestamp, latency_ms, status_code, error)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range events {
		errBit := 0
		if e.Error {
			errBit = 1
		}
		if _, err := stmt.Exec(e.ProjectID, e.Timestamp.Unix(), e.LatencyMS, e.StatusCode, errBit); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}
	return tx.Commit()
}

// InsertMetricsBatch inserts many pre-aggregated buckets in one transaction —
// for seeding historical data directly, bypassing the normal raw->rollup path.
func (s *Store) InsertMetricsBatch(metrics []models.Metric) error {
	if len(metrics) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO aggregated_metrics (project_id, bucket_start, resolution, good_events, total_events)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, m := range metrics {
		if _, err := stmt.Exec(m.ProjectID, m.BucketStart.Unix(), m.Resolution, m.GoodEvents, m.TotalEvents); err != nil {
			return fmt.Errorf("insert metric: %w", err)
		}
	}
	return tx.Commit()
}

// RunDownsampleLoop calls Downsample on an interval until ctx-like stop signal.
// Kept dumb on purpose: caller decides cadence.
func (s *Store) RunDownsampleLoop(every time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			if err := s.Downsample(now); err != nil {
				log.Printf("downsample: %v", err)
			}
		}
	}
}
