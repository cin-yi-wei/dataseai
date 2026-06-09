package oracle

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSN(t *testing.T) {
	o := Oracle{}
	dsn := o.BuildDSN(db.DSNInput{Host: "dbhost", Port: 1521, Username: "scott", Password: "tiger", DefaultDB: "ORCL"})
	if !strings.Contains(dsn, "dbhost") {
		t.Errorf("missing host: %q", dsn)
	}

	// Network override (SSH forwarder)
	dsn2 := o.BuildDSN(db.DSNInput{Network: "127.0.0.1:12345", Host: "ignored", Port: 1521, DefaultDB: "ORCL"})
	if !strings.Contains(dsn2, "127.0.0.1") {
		t.Errorf("expected forwarder addr, got %q", dsn2)
	}
	if strings.Contains(dsn2, "ignored") {
		t.Errorf("DSN should not contain original host: %q", dsn2)
	}
}

func TestSplitHostPort(t *testing.T) {
	h, p := splitHostPort("127.0.0.1:5555", 1521)
	if h != "127.0.0.1" || p != 5555 {
		t.Errorf("got %s:%d", h, p)
	}
	h2, p2 := splitHostPort("noport", 1521)
	if h2 != "noport" || p2 != 1521 {
		t.Errorf("got %s:%d", h2, p2)
	}
}
