package tidb

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

var _ db.Dialect = TiDB{}

func TestEngine(t *testing.T) {
	d := TiDB{}
	if d.Engine() != db.EngineTiDB {
		t.Errorf("Engine() = %q, want %q", d.Engine(), db.EngineTiDB)
	}
}

func TestDriverName(t *testing.T) {
	d := TiDB{}
	if d.DriverName() != "mysql" {
		t.Errorf("DriverName() = %q, want mysql", d.DriverName())
	}
}

func TestBuildDSN(t *testing.T) {
	d := TiDB{}
	dsn := d.BuildDSN(db.DSNInput{Host: "tidb-host", Port: 4000, Username: "root", Password: "pw"})
	for _, want := range []string{"tidb-host:4000", "root:pw"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN %q missing %q", dsn, want)
		}
	}
}
