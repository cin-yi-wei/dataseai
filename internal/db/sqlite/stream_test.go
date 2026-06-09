package sqlite

import (
	"context"
	"testing"
)

func TestStreamQueryIntegration(t *testing.T) {
	sdb := openMemDB(t)
	ctx := context.Background()
	if _, err := sdb.ExecContext(ctx, "CREATE TABLE nums (n INTEGER)"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := sdb.ExecContext(ctx, "INSERT INTO nums VALUES (?)", i); err != nil {
			t.Fatal(err)
		}
	}

	var gotCols []string
	var gotRows [][]any
	var gotTotal int64

	err := StreamQuery(ctx, sdb, "SELECT * FROM nums ORDER BY n", StreamOpts{BatchSize: 2}, StreamSink{
		Columns: func(cols []string) { gotCols = cols },
		Batch:   func(rows [][]any, _ int) error { gotRows = append(gotRows, rows...); return nil },
		Done:    func(total int64) { gotTotal = total },
	})
	if err != nil {
		t.Fatalf("StreamQuery: %v", err)
	}
	if len(gotCols) != 1 || gotCols[0] != "n" {
		t.Errorf("cols = %v", gotCols)
	}
	if len(gotRows) != 5 {
		t.Errorf("got %d rows, want 5", len(gotRows))
	}
	if gotTotal != 5 {
		t.Errorf("total = %d, want 5", gotTotal)
	}
}

func TestStreamQueryEmpty(t *testing.T) {
	sdb := openMemDB(t)
	ctx := context.Background()
	if _, err := sdb.ExecContext(ctx, "CREATE TABLE empty (x TEXT)"); err != nil {
		t.Fatal(err)
	}
	var gotTotal int64 = -1
	err := StreamQuery(ctx, sdb, "SELECT * FROM empty", StreamOpts{}, StreamSink{
		Done: func(total int64) { gotTotal = total },
	})
	if err != nil {
		t.Fatalf("StreamQuery: %v", err)
	}
	if gotTotal != 0 {
		t.Errorf("total = %d, want 0", gotTotal)
	}
}
