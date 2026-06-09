package bytehouse

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialerRejectsZero(t *testing.T) {
	d := BH{}
	if _, _, err := d.RegisterSSHDialer(db.SSHConfig{}, db.DSNInput{}); err == nil {
		t.Fatal("expected error for zero SSHConfig")
	}
}
