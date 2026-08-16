package storage

import (
	"database/sql"
	"fmt"
	"log"
)

type Migration struct {
	Version     int
	Description string
	SQL         string
}

// Migrations lists every schema change in order. To change the schema, add a
// new entry here — never edit an already-shipped one.
var Migrations = []Migration{
	{
		Version:     1,
		Description: "original schema: raw_events, aggregated_metrics, slo_alerts",
		SQL: `
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
    project_id  TEXT    NOT NULL,
    fired_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_slo_alerts_proj_ts ON slo_alerts(project_id, fired_at);
`,
	},
	{
		Version:     2,
		Description: "add issue_number, resolved_at, budget_consumed_pct to slo_alerts",
		SQL: `
ALTER TABLE slo_alerts ADD COLUMN issue_number INTEGER NOT NULL DEFAULT 0;
ALTER TABLE slo_alerts ADD COLUMN resolved_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE slo_alerts ADD COLUMN budget_consumed_pct REAL NOT NULL DEFAULT 0;
`,
	},
	{
		Version:     3,
		Description: "add short_burn_rate, long_burn_rate, slo_pct, target_pct to slo_alerts",
		SQL: `
ALTER TABLE slo_alerts ADD COLUMN short_burn_rate REAL NOT NULL DEFAULT 0;
ALTER TABLE slo_alerts ADD COLUMN long_burn_rate REAL NOT NULL DEFAULT 0;
ALTER TABLE slo_alerts ADD COLUMN slo_pct REAL NOT NULL DEFAULT 0;
ALTER TABLE slo_alerts ADD COLUMN target_pct REAL NOT NULL DEFAULT 0;
`,
	},
	// Version 4: eventQueue async write batching — Go-side only, no schema change.
}

const migrationsTableSchema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    description TEXT
)`

// RunMigrations brings db's schema up to date by applying, in order, every
// Migration whose version is greater than the highest one already recorded
// in schema_migrations. Safe to call on every startup.
func RunMigrations(db *sql.DB) error {
	if _, err := db.Exec(migrationsTableSchema); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	var current int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("query current schema version: %w", err)
	}

	// A DB that predates the migration system may already carry the columns
	// migrations 2 and 3 add — they ran as raw ALTER TABLE guards before
	// schema_migrations existed. ADD COLUMN has no IF NOT EXISTS in SQLite,
	// so re-running them fails with "duplicate column name"; detect that
	// state and backfill the tracking rows instead of re-applying the SQL.
	// Migration 1 (CREATE TABLE/INDEX IF NOT EXISTS) is left to run through
	// the normal loop below — it's idempotent against the existing tables
	// and still needs to be recorded.
	backfilled := make(map[int]bool)
	if current == 0 {
		hasIssueNumber, err := sloAlertsHasColumn(db, "issue_number")
		if err != nil {
			return fmt.Errorf("check pre-migration schema: %w", err)
		}
		if hasIssueNumber {
			for _, v := range []int{2, 3} {
				log.Printf("migration %d already applied via pre-migration ALTER TABLE guards; backfilling schema_migrations", v)
				if _, err := db.Exec(
					`INSERT INTO schema_migrations (version, description) VALUES (?, ?)`,
					v, descriptionForVersion(v),
				); err != nil {
					return fmt.Errorf("backfill migration %d: %w", v, err)
				}
				backfilled[v] = true
			}
		}
	}

	for _, m := range Migrations {
		if m.Version <= current || backfilled[m.Version] {
			continue
		}
		log.Printf("applying migration %d: %s", m.Version, m.Description)

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version, description) VALUES (?, ?)`,
			m.Version, m.Description,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}
	return nil
}

// sloAlertsHasColumn reports whether slo_alerts already has the given
// column. False (with no error) if the table doesn't exist yet — a fresh DB
// just takes the normal migration path.
func sloAlertsHasColumn(db *sql.DB, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(slo_alerts)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func descriptionForVersion(version int) string {
	for _, m := range Migrations {
		if m.Version == version {
			return m.Description
		}
	}
	return ""
}
