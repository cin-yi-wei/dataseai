package db

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestDirectExecutorRunsSelect(t *testing.T) {
	dbh, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer dbh.Close()
	if _, err := dbh.Exec("CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	exec := DirectExecutor{DB: dbh}
	res, err := exec.Run(context.Background(), "SELECT id FROM t")
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != 0 {
		t.Fatalf("RowsAffected = %d, want 0 for select", res.RowsAffected)
	}
}
