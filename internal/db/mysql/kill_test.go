package mysql

import (
	"testing"
)

func TestConnectionIDQuery(t *testing.T) {
	if got := (MySQL{}).ConnectionIDQuery(); got != "SELECT CONNECTION_ID()" {
		t.Fatalf("ConnectionIDQuery = %q, want %q", got, "SELECT CONNECTION_ID()")
	}
}
