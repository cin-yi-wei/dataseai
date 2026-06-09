package clickhouse

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSN(t *testing.T) {
	c := CH{}

	dsn := c.BuildDSN(db.DSNInput{Host: "localhost", Port: 9000, Username: "user", Password: "pass", DefaultDB: "mydb"})
	if !strings.HasPrefix(dsn, "clickhouse://") {
		t.Errorf("expected clickhouse:// scheme, got %q", dsn)
	}
	if !strings.Contains(dsn, "localhost:9000") {
		t.Errorf("missing host:port in %q", dsn)
	}

	// Network override
	dsn2 := c.BuildDSN(db.DSNInput{Network: "127.0.0.1:12345", Host: "ignored", Port: 9000})
	if !strings.Contains(dsn2, "127.0.0.1:12345") {
		t.Errorf("expected forwarded addr, got %q", dsn2)
	}
}
