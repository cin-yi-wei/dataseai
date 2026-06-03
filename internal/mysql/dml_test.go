package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupDMLSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t(name, email) VALUES ('alice','a@x'), ('bob','b@x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE u (name TEXT, email TEXT)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPrimaryKey_ReturnsSQLiteColumns(t *testing.T) {
	db := setupDMLSQLite(t)
	got, err := PrimaryKey(context.Background(), db, "", "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "id" {
		t.Fatalf("pk = %+v, want [id]", got)
	}
}

func TestUpdateCell_RejectsNoPK(t *testing.T) {
	db := setupDMLSQLite(t)
	_, err := UpdateCell(context.Background(), db, "", "u", []string{}, []any{}, "name", "x")
	if !errors.Is(err, ErrNoPrimaryKey) {
		t.Fatalf("want ErrNoPrimaryKey, got %v", err)
	}
}

func TestUpdateCell_HappyPath(t *testing.T) {
	db := setupDMLSQLite(t)
	n, err := UpdateCell(context.Background(), db, "", "t", []string{"id"}, []any{int64(1)}, "name", "ALICE")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if n != 1 {
		t.Fatalf("affected = %d, want 1", n)
	}
	var name string
	if err := db.QueryRow("SELECT name FROM t WHERE id=1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "ALICE" {
		t.Fatalf("name = %q", name)
	}
}

func TestInsertRow(t *testing.T) {
	db := setupDMLSQLite(t)
	id, err := InsertRow(context.Background(), db, "", "t", []string{"name", "email"}, []any{"cathy", "c@x"})
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("id = %d", id)
	}
}

func TestDeleteRow_RejectsNoPK(t *testing.T) {
	db := setupDMLSQLite(t)
	_, err := DeleteRow(context.Background(), db, "", "u", []string{}, []any{})
	if !errors.Is(err, ErrNoPrimaryKey) {
		t.Fatalf("want ErrNoPrimaryKey, got %v", err)
	}
}

func TestDeleteRow_HappyPath(t *testing.T) {
	db := setupDMLSQLite(t)
	n, err := DeleteRow(context.Background(), db, "", "t", []string{"id"}, []any{int64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("affected = %d, want 1", n)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM t WHERE id=2").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("row still exists")
	}
}
