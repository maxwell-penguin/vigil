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
