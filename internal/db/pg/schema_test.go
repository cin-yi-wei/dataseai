package pg

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestMarkPrimaryKeyColumns(t *testing.T) {
	cols := []db.Column{
		{Name: "id"},
		{Name: "email"},
	}

	markPrimaryKeyColumns(cols, []string{"id"})

	if cols[0].Key != "PRI" {
		t.Fatalf("id key = %q, want PRI", cols[0].Key)
	}
	if cols[1].Key != "" {
		t.Fatalf("email key = %q, want empty", cols[1].Key)
	}
}
