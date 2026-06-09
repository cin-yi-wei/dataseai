package clickhouse

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialer_badConfig(t *testing.T) {
	c := CH{}
	_, _, err := c.RegisterSSHDialer(db.SSHConfig{}, db.DSNInput{})
	if err == nil {
		t.Fatal("expected error for empty SSH config")
	}
}
