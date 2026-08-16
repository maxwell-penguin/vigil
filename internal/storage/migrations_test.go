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

// TestRunMigrations_PreExistingColumns reproduces the live Fly.io DB: a
// database that predates the migration system, whose slo_alerts table
// already has the columns migrations 2 and 3 would add (from the old
// ALTER TABLE guards), but with no schema_migrations table at all.
// RunMigrations must backfill tracking rows instead of re-running the ALTER
// TABLE statements and failing with "duplicate column name".
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
		if description != descriptionForVersion(v) {
			t.Errorf("version %d: description = %q, want %q", v, description, descriptionForVersion(v))
		}
	}

	// Re-running must still be a no-op (idempotency after backfill).
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
