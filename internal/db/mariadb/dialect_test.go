package mariadb

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

// compile-time: MariaDB satisfies db.Dialect.
var _ db.Dialect = MariaDB{}

func TestEngine(t *testing.T) {
	d := MariaDB{}
	if d.Engine() != db.EngineMariaDB {
		t.Errorf("Engine() = %q, want %q", d.Engine(), db.EngineMariaDB)
	}
}

func TestDriverName(t *testing.T) {
	d := MariaDB{}
	if d.DriverName() != "mysql" {
		t.Errorf("DriverName() = %q, want mysql", d.DriverName())
	}
}

func TestBuildDSNHasHost(t *testing.T) {
	d := MariaDB{}
	dsn := d.BuildDSN(db.DSNInput{Host: "mariadb-host", Port: 3306, Username: "root", Password: "pw"})
	for _, want := range []string{"mariadb-host:3306", "root:pw"} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN %q missing %q", dsn, want)
		}
	}
}
