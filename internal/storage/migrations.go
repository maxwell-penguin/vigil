package storage

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
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

	for _, m := range Migrations {
		if m.Version <= current {
			continue
		}
		log.Printf("applying migration %d: %s", m.Version, m.Description)

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}

		stmts := splitStatements(m.SQL)
		if isAlterTableOnly(stmts) {
			// ADD COLUMN has no IF NOT EXISTS in SQLite. DBs that predate
			// the migration system (or a prior run that got partway
			// through) may already have some or all of these columns —
			// run each statement on its own and tolerate "duplicate
			// column name" rather than failing the whole migration.
			for _, stmt := range stmts {
				if err := addColumnIfNotExists(tx, stmt); err != nil {
					tx.Rollback()
					return fmt.Errorf("apply migration %d: %w", m.Version, err)
				}
			}
		} else if _, err := tx.Exec(m.SQL); err != nil {
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

// addColumnIfNotExists runs a single ALTER TABLE ... ADD COLUMN statement,
// treating "duplicate column name" as success since that just means the
// column is already there.
func addColumnIfNotExists(tx *sql.Tx, stmt string) error {
	_, err := tx.Exec(stmt)
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}

// splitStatements breaks a semicolon-separated block of SQL into its
// individual statements, dropping empty ones.
func splitStatements(sql string) []string {
	var stmts []string
	for _, stmt := range strings.Split(sql, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt != "" {
			stmts = append(stmts, stmt)
		}
	}
	return stmts
}

// isAlterTableOnly reports whether every statement is an ALTER TABLE —
// migrations 2 and 3 are ADD COLUMN-only, so they run through the
// duplicate-tolerant path instead of a single all-or-nothing exec.
func isAlterTableOnly(stmts []string) bool {
	if len(stmts) == 0 {
		return false
	}
	for _, stmt := range stmts {
		if !strings.HasPrefix(strings.ToUpper(stmt), "ALTER TABLE") {
			return false
		}
	}
	return true
}
