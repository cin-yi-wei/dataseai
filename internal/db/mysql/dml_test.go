package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/conray/dataseai/internal/db"
)

func setupDMLSQLite(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if _, err := sqlDB.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO t(name, email) VALUES ('alice','a@x'), ('bob','b@x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`CREATE TABLE u (name TEXT, email TEXT)`); err != nil {
		t.Fatal(err)
	}
	return sqlDB
}

func TestPrimaryKey_ReturnsSQLiteColumns(t *testing.T) {
	sqlDB := setupDMLSQLite(t)
	got, err := MySQL{}.PrimaryKey(context.Background(), sqlDB, "", "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "id" {
		t.Fatalf("pk = %+v, want [id]", got)
	}
}

func TestUpdateCell_RejectsNoPK(t *testing.T) {
	sqlDB := setupDMLSQLite(t)
	_, err := MySQL{}.UpdateCell(context.Background(), sqlDB, "", "u", []string{}, []any{}, "name", "x")
	if !errors.Is(err, db.ErrNoPrimaryKey) {
		t.Fatalf("want ErrNoPrimaryKey, got %v", err)
	}
}

func TestUpdateCell_HappyPath(t *testing.T) {
	sqlDB := setupDMLSQLite(t)
	n, err := MySQL{}.UpdateCell(context.Background(), sqlDB, "", "t", []string{"id"}, []any{int64(1)}, "name", "ALICE")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if n != 1 {
		t.Fatalf("affected = %d, want 1", n)
	}
	var name string
	if err := sqlDB.QueryRow("SELECT name FROM t WHERE id=1").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "ALICE" {
		t.Fatalf("name = %q", name)
	}
}

func TestInsertRow(t *testing.T) {
	sqlDB := setupDMLSQLite(t)
	id, err := MySQL{}.InsertRow(context.Background(), sqlDB, "", "t", []string{"name", "email"}, []any{"cathy", "c@x"})
	if err != nil {
		t.Fatal(err)
	}
	if id <= 0 {
		t.Fatalf("id = %d", id)
	}
}

func TestDeleteRow_RejectsNoPK(t *testing.T) {
	sqlDB := setupDMLSQLite(t)
	_, err := MySQL{}.DeleteRow(context.Background(), sqlDB, "", "u", []string{}, []any{})
	if !errors.Is(err, db.ErrNoPrimaryKey) {
		t.Fatalf("want ErrNoPrimaryKey, got %v", err)
	}
}

func TestDeleteRow_HappyPath(t *testing.T) {
	sqlDB := setupDMLSQLite(t)
	n, err := MySQL{}.DeleteRow(context.Background(), sqlDB, "", "t", []string{"id"}, []any{int64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("affected = %d, want 1", n)
	}
	var count int
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM t WHERE id=2").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("row still exists")
	}
}

func TestUpdateCellByMatch_NoPK(t *testing.T) {
	ctx := context.Background()
	sqlDB := setupDMLSQLite(t)
	// u has no primary key. Seed distinct rows, a NULL row, and a duplicate pair.
	if _, err := sqlDB.Exec(`INSERT INTO u(name,email) VALUES ('x','x@x'),('n',NULL),('dup','d@x'),('dup','d@x')`); err != nil {
		t.Fatal(err)
	}

	// Unique match → updates exactly one row.
	n, err := MySQL{}.UpdateCellByMatch(ctx, sqlDB, "", "u", []string{"name", "email"}, []any{"x", "x@x"}, "email", "x2@x")
	if err != nil || n != 1 {
		t.Fatalf("unique update: n=%d err=%v", n, err)
	}

	// NULL column matched via IS NULL.
	n, err = MySQL{}.UpdateCellByMatch(ctx, sqlDB, "", "u", []string{"name", "email"}, []any{"n", nil}, "email", "filled@x")
	if err != nil || n != 1 {
		t.Fatalf("null-match update: n=%d err=%v", n, err)
	}

	// Duplicate rows → refuse (ambiguous), touch nothing.
	_, err = MySQL{}.UpdateCellByMatch(ctx, sqlDB, "", "u", []string{"name", "email"}, []any{"dup", "d@x"}, "email", "nope")
	if !errors.Is(err, db.ErrAmbiguousRow) {
		t.Fatalf("want ErrAmbiguousRow, got %v", err)
	}
	var c int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM u WHERE email='nope'`).Scan(&c)
	if c != 0 {
		t.Fatalf("ambiguous update should not have changed rows")
	}

	// Delete by match: unique → 1; duplicate → refuse.
	n, err = MySQL{}.DeleteRowByMatch(ctx, sqlDB, "", "u", []string{"name", "email"}, []any{"x", "x2@x"})
	if err != nil || n != 1 {
		t.Fatalf("unique delete: n=%d err=%v", n, err)
	}
	if _, err := (MySQL{}).DeleteRowByMatch(ctx, sqlDB, "", "u", []string{"name", "email"}, []any{"dup", "d@x"}); !errors.Is(err, db.ErrAmbiguousRow) {
		t.Fatalf("want ErrAmbiguousRow on delete, got %v", err)
	}
}

func TestCoerceValueISODateTime(t *testing.T) {
	cases := []struct {
		in   any
		want any
	}{
		{"2026-06-03T04:05:48.280689Z", "2026-06-03 04:05:48.280689"},
		{"2026-06-03T04:05:48Z", "2026-06-03 04:05:48"},
		{"2026-06-03T04:05:48", "2026-06-03 04:05:48"},
		{"2026-06-03T04:05:48+08:00", "2026-06-02 20:05:48"},
		{"hello", "hello"},
		{"2026-06-03", "2026-06-03"},
		{42, 42},
		{nil, nil},
	}
	for _, c := range cases {
		got := coerceValue(c.in)
		if got != c.want {
			t.Errorf("coerceValue(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
