package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupRowsSQLite(t *testing.T, n int) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE r (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		if _, err := db.Exec("INSERT INTO r(v) VALUES(?)", "row-"+strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestStreamQuery_DeliversBatches(t *testing.T) {
	db := setupRowsSQLite(t, 250)
	got := struct {
		cols    []string
		batches int
		rows    int
		done    bool
	}{}
	err := StreamQuery(context.Background(), db, "SELECT id, v FROM r ORDER BY id", StreamOpts{BatchSize: 100}, StreamSink{
		Columns: func(c []string) { got.cols = c },
		Batch: func(rows [][]any, offset int) error {
			if offset != got.rows {
				t.Fatalf("offset = %d, want %d", offset, got.rows)
			}
			got.batches++
			got.rows += len(rows)
			return nil
		},
		Done: func(total int64) { got.done = true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.done || got.rows != 250 || got.batches != 3 || len(got.cols) != 2 {
		t.Fatalf("got = %+v", got)
	}
}

func TestStreamQuery_CancelStopsEarly(t *testing.T) {
	db := setupRowsSQLite(t, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	rowsSeen := 0
	err := StreamQuery(ctx, db, "SELECT id, v FROM r", StreamOpts{BatchSize: 50}, StreamSink{
		Batch: func(rows [][]any, offset int) error {
			rowsSeen += len(rows)
			if rowsSeen >= 100 {
				cancel()
			}
			return nil
		},
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if rowsSeen < 100 || rowsSeen > 200 {
		t.Fatalf("seen %d rows; expected around 100", rowsSeen)
	}
}
