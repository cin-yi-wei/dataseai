package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestSQLite(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestExecute_ListDatabases(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	// Without information_schema, this will fail — capture that.
	out, err := Execute(context.Background(), ExecCtx{DB: db}, "list_databases", map[string]any{})
	if err == nil {
		t.Skip("sqlite happens to have information_schema-like view")
	}
	if out != "" {
		t.Fatalf("out should be empty on error, got %q", out)
	}
}

func TestExecute_RunSQL(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	_, _ = db.Exec("CREATE TABLE t(id INT, n TEXT); INSERT INTO t VALUES(1,'a'),(2,'b')")
	out, err := Execute(context.Background(), ExecCtx{DB: db}, "run_sql", map[string]any{"sql": "SELECT * FROM t ORDER BY id"})
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if parsed["rows"] == nil {
		t.Fatalf("missing rows: %s", out)
	}
}

func TestExecute_UnknownTool(t *testing.T) {
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()
	_, err := Execute(context.Background(), ExecCtx{DB: db}, "no_such_tool", map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunSQLBlocksDML(t *testing.T) {
	ctx := context.Background()
	db := setupTestSQLite(t)
	out, err := Execute(ctx, ExecCtx{DB: db}, "run_sql", map[string]any{"sql": "DELETE FROM t WHERE id=1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(out, "run_sql_readonly") {
		t.Fatalf("expected reject, got %s", out)
	}
}

func TestRunSQLAllowsExplain(t *testing.T) {
	ctx := context.Background()
	db := setupTestSQLite(t)
	out, err := Execute(ctx, ExecCtx{DB: db}, "run_sql", map[string]any{"sql": "EXPLAIN SELECT * FROM t"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.Contains(out, "run_sql_readonly") {
		t.Fatalf("EXPLAIN should pass, got %s", out)
	}
}
