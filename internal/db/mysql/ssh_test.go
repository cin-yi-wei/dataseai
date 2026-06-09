package mysql

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialerRejectsZero(t *testing.T) {
	d := MySQL{}
	if _, _, err := d.RegisterSSHDialer(db.SSHConfig{}); err == nil {
		t.Fatal("expected error for zero SSHConfig")
	}
}
