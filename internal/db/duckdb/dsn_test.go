package duckdb

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSN(t *testing.T) {
	d := DuckDB{}
	if got := d.BuildDSN(db.DSNInput{Host: "/data/mydb.duckdb"}); got != "/data/mydb.duckdb" {
		t.Errorf("got %q", got)
	}
	if got := d.BuildDSN(db.DSNInput{Host: ":memory:"}); got != ":memory:" {
		t.Errorf("got %q", got)
	}
	if got := d.BuildDSN(db.DSNInput{Network: "override", Host: "/ignored"}); got != "override" {
		t.Errorf("got %q", got)
	}
}
