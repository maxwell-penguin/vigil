package storage

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestRunMigrations_FreshDB(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != len(Migrations) {
		t.Fatalf("schema_migrations has %d rows, want %d", count, len(Migrations))
	}

	rows, err := db.Query(`SELECT version, description FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		var version int
		var description string
		if err := rows.Scan(&version, &description); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if version != Migrations[i].Version {
			t.Errorf("row %d: version = %d, want %d", i, version, Migrations[i].Version)
		}
		if description != Migrations[i].Description {
			t.Errorf("row %d: description = %q, want %q", i, description, Migrations[i].Description)
		}
		i++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != len(Migrations) {
		t.Fatalf("schema_migrations has %d rows after re-run, want %d", count, len(Migrations))
	}
}

func descriptionForVersion(t *testing.T, version int) string {
	t.Helper()
	for _, m := range Migrations {
		if m.Version == version {
			return m.Description
		}
	}
	t.Fatalf("no migration with version %d", version)
	return ""
}

// TestRunMigrations_PreExistingColumns reproduces the live Fly.io DB: a
// database that predates the migration system, whose slo_alerts table
// already has the columns migrations 2 and 3 would add (from the old
// ALTER TABLE guards), but with no schema_migrations table at all.
// RunMigrations must tolerate "duplicate column name" per-statement instead
// of failing the whole migration.
func TestRunMigrations_PreExistingColumns(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.Exec(`
CREATE TABLE raw_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT    NOT NULL,
    timestamp   INTEGER NOT NULL,
    latency_ms  INTEGER NOT NULL,
    status_code INTEGER NOT NULL,
    error       INTEGER NOT NULL
);
CREATE TABLE aggregated_metrics (
    project_id   TEXT    NOT NULL,
    bucket_start INTEGER NOT NULL,
    resolution   TEXT    NOT NULL,
    good_events  INTEGER NOT NULL,
    total_events INTEGER NOT NULL,
    PRIMARY KEY (project_id, bucket_start, resolution)
);
CREATE TABLE slo_alerts (
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
`); err != nil {
		t.Fatalf("seed pre-migration schema: %v", err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations on pre-migration DB: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != len(Migrations) {
		t.Fatalf("schema_migrations has %d rows, want %d", count, len(Migrations))
	}

	for _, v := range []int{2, 3} {
		var version int
		var description string
		if err := db.QueryRow(`SELECT version, description FROM schema_migrations WHERE version = ?`, v).
			Scan(&version, &description); err != nil {
			t.Fatalf("row for version %d: %v", v, err)
		}
		if want := descriptionForVersion(t, v); description != want {
			t.Errorf("version %d: description = %q, want %q", v, description, want)
		}
	}

	// Re-running must still be a no-op.
	if err := RunMigrations(db); err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations after re-run: %v", err)
	}
	if count != len(Migrations) {
		t.Fatalf("schema_migrations has %d rows after re-run, want %d", count, len(Migrations))
	}
}

// TestRunMigrations_PartialFailureRetry reproduces the actual production
// crash: migration 1 committed and was recorded (current = 1), but the
// slo_alerts table already carried migration 2 and 3's columns (from the
// pre-migration ALTER TABLE guards), so a naive re-run of migration 2 hit
// "duplicate column name" on every retry — current never drops back to 0,
// so any fix gated on "current == 0" never engages. RunMigrations must
// succeed regardless of what current is.
func TestRunMigrations_PartialFailureRetry(t *testing.T) {
	db := openTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("seed via first RunMigrations: %v", err)
	}
	// Roll schema_migrations back to just after migration 1, as if 2 and 3
	// never got recorded — the columns are still there from migration 1's
	// original run (or pre-migration guards), only the bookkeeping regressed.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version > 1`); err != nil {
		t.Fatalf("simulate partial failure: %v", err)
	}

	var current int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&current); err != nil {
		t.Fatalf("read current version: %v", err)
	}
	if current != 1 {
		t.Fatalf("test setup: current = %d, want 1", current)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations after partial failure: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count != len(Migrations) {
		t.Fatalf("schema_migrations has %d rows, want %d", count, len(Migrations))
	}
}
