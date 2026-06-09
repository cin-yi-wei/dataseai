package store

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrate_AppliesAllAndIsIdempotent(t *testing.T) {
	db := openMem(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// run twice — should be a no-op
	if err := Migrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 1 {
		t.Fatalf("expected at least 1 migration applied, got %d", n)
	}
}

func TestMigrationAddsEngineColumn(t *testing.T) {
	// Open a fresh in-memory store and verify the engine column exists with the default 'mysql'.
	db := openMem(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rows, err := db.Query("PRAGMA table_info(connections)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "engine" {
			found = true
			if !dflt.Valid || dflt.String != "'mysql'" {
				t.Fatalf("engine default = %v, want 'mysql'", dflt)
			}
			if notnull == 0 {
				t.Fatalf("engine column should be NOT NULL")
			}
		}
	}
	if !found {
		t.Fatal("engine column missing")
	}

	// index exists
	var idxName string
	if err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_connections_engine'`,
	).Scan(&idxName); err != nil {
		t.Fatalf("idx_connections_engine missing: %v", err)
	}
}

func TestMigrate0009AIWrites(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	// users.ai_writes_enabled exists
	var cnt int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('users') WHERE name='ai_writes_enabled'`).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("ai_writes_enabled column missing, got %d", cnt)
	}

	// policy + audit tables present
	for _, tbl := range []string{"ai_write_policy", "ai_write_audit"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}
