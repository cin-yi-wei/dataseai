package snowflake

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestRegisterSSHDialer(t *testing.T) {
	s := Snowflake{}
	_, _, err := s.RegisterSSHDialer(db.SSHConfig{}, db.DSNInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}
