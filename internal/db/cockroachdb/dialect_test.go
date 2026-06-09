package cockroachdb

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

var _ db.Dialect = CockroachDB{}

func TestEngine(t *testing.T) {
	d := CockroachDB{}
	if d.Engine() != db.EngineCockroachDB {
		t.Errorf("Engine() = %q, want %q", d.Engine(), db.EngineCockroachDB)
	}
}

func TestDriverName(t *testing.T) {
	d := CockroachDB{}
	if d.DriverName() != "pgx" {
		t.Errorf("DriverName() = %q, want pgx", d.DriverName())
	}
}

func TestBuildDSN(t *testing.T) {
	d := CockroachDB{}
	dsn := d.BuildDSN(db.DSNInput{Host: "crdb-host", Port: 26257, Username: "root", Password: "pw", DefaultDB: "defaultdb"})
	for _, want := range []string{"crdb-host", "26257", "root"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN %q missing %q", dsn, want)
		}
	}
}
