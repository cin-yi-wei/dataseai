package redshift

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

var _ db.Dialect = Redshift{}

func TestEngine(t *testing.T) {
	d := Redshift{}
	if d.Engine() != db.EngineRedshift {
		t.Errorf("Engine() = %q, want %q", d.Engine(), db.EngineRedshift)
	}
}

func TestDriverName(t *testing.T) {
	d := Redshift{}
	if d.DriverName() != "pgx" {
		t.Errorf("DriverName() = %q, want pgx", d.DriverName())
	}
}

func TestBuildDSN(t *testing.T) {
	d := Redshift{}
	dsn := d.BuildDSN(db.DSNInput{Host: "cluster.redshift.amazonaws.com", Port: 5439, Username: "admin", DefaultDB: "dev"})
	for _, want := range []string{"cluster.redshift.amazonaws.com", "5439", "admin", "dev"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN %q missing %q", dsn, want)
		}
	}
}
