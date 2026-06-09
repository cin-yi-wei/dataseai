package sqlite

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialerNotSupported(t *testing.T) {
	d := SQLite{}
	_, _, err := d.RegisterSSHDialer(db.SSHConfig{Host: "h", User: "u", Port: 22}, db.DSNInput{})
	if err == nil {
		t.Fatal("expected error for SSH on SQLite, got nil")
	}
}
