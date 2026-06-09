package sqlite

import (
	"context"
	"database/sql"
	"testing"
)

func openMemDB(t *testing.T) *sql.DB {
	t.Helper()
	sdb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open :memory: failed: %v", err)
	}
	t.Cleanup(func() { sdb.Close() })
	return sdb
}

func TestPrimaryKeyIntegration(t *testing.T) {
	sdb := openMemDB(t)
	ctx := context.Background()
	if _, err := sdb.ExecContext(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	d := SQLite{}
	pks, err := d.PrimaryKey(ctx, sdb, "main", "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(pks) != 1 || pks[0] != "id" {
		t.Errorf("PrimaryKey = %v, want [id]", pks)
	}
}

func TestPrimaryKeyNoPK(t *testing.T) {
	sdb := openMemDB(t)
	ctx := context.Background()
	if _, err := sdb.ExecContext(ctx, "CREATE TABLE nopk (a TEXT, b TEXT)"); err != nil {
		t.Fatal(err)
	}
	d := SQLite{}
	pks, err := d.PrimaryKey(ctx, sdb, "main", "nopk")
	if err != nil {
		t.Fatal(err)
	}
	if len(pks) != 0 {
		t.Errorf("expected no PK cols, got %v", pks)
	}
}

func TestInsertUpdateDeleteIntegration(t *testing.T) {
	sdb := openMemDB(t)
	ctx := context.Background()
	if _, err := sdb.ExecContext(ctx, "CREATE TABLE items (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		t.Fatal(err)
	}
	d := SQLite{}

	id, err := d.InsertRow(ctx, sdb, "main", "items", []string{"id", "val"}, []any{1, "hello"})
	if err != nil {
		t.Fatalf("InsertRow: %v", err)
	}
	if id != 1 {
		t.Errorf("InsertRow returned id=%d, want 1", id)
	}

	n, err := d.UpdateCell(ctx, sdb, "main", "items", []string{"id"}, []any{1}, "val", "world")
	if err != nil {
		t.Fatalf("UpdateCell: %v", err)
	}
	if n != 1 {
		t.Errorf("UpdateCell affected %d rows, want 1", n)
	}

	var got string
	if err := sdb.QueryRowContext(ctx, "SELECT val FROM items WHERE id=1").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "world" {
		t.Errorf("val after update = %q, want world", got)
	}

	n, err = d.DeleteRow(ctx, sdb, "main", "items", []string{"id"}, []any{1})
	if err != nil {
		t.Fatalf("DeleteRow: %v", err)
	}
	if n != 1 {
		t.Errorf("DeleteRow affected %d rows, want 1", n)
	}
}
