package oracle

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialer_badConfig(t *testing.T) {
	o := Oracle{}
	_, _, err := o.RegisterSSHDialer(db.SSHConfig{}, db.DSNInput{})
	if err == nil {
		t.Fatal("expected error for empty SSH config")
	}
}
