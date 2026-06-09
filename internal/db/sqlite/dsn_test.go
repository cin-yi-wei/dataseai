package sqlite

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSNFilePath(t *testing.T) {
	d := SQLite{}
	got := d.BuildDSN(db.DSNInput{Host: "/var/data/app.db"})
	if got != "/var/data/app.db" {
		t.Fatalf("expected file path, got %q", got)
	}
}

func TestBuildDSNMemory(t *testing.T) {
	d := SQLite{}
	got := d.BuildDSN(db.DSNInput{Host: ":memory:"})
	if got != ":memory:" {
		t.Fatalf("expected :memory:, got %q", got)
	}
}

func TestBuildDSNNetworkPassthrough(t *testing.T) {
	d := SQLite{}
	in := db.DSNInput{Host: "/original.db", Network: "/override.db"}
	if got := d.BuildDSN(in); got != "/override.db" {
		t.Fatalf("network passthrough failed: %q", got)
	}
}
