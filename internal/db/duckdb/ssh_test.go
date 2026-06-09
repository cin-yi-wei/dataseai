package duckdb

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialer(t *testing.T) {
	d := DuckDB{}
	_, _, err := d.RegisterSSHDialer(db.SSHConfig{}, db.DSNInput{})
	if err == nil {
		t.Fatal("expected error for SSH on file-based DB")
	}
}
