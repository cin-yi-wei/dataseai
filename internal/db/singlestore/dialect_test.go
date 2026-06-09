package singlestore

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

var _ db.Dialect = SingleStore{}

func TestEngine(t *testing.T) {
	d := SingleStore{}
	if d.Engine() != db.EngineSingleStore {
		t.Errorf("Engine() = %q, want %q", d.Engine(), db.EngineSingleStore)
	}
}

func TestDriverName(t *testing.T) {
	d := SingleStore{}
	if d.DriverName() != "mysql" {
		t.Errorf("DriverName() = %q, want mysql", d.DriverName())
	}
}

func TestBuildDSN(t *testing.T) {
	d := SingleStore{}
	dsn := d.BuildDSN(db.DSNInput{Host: "ss-host", Port: 3306, Username: "admin", DefaultDB: "mydb"})
	for _, want := range []string{"ss-host:3306", "admin", "mydb"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN %q missing %q", dsn, want)
		}
	}
}
