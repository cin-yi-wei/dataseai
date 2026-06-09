package mysql

import (
	"context"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestMySQLImplementsSchemaMethods(t *testing.T) {
	var d db.Dialect = MySQL{}
	// Compile-time check that types match the interface.
	_ = d
	// Call signatures — passing nil ctx + nil *sql.DB will likely panic at
	// runtime, so don't actually invoke. The interface satisfaction itself
	// is what we assert here.
	_ = func(ctx context.Context) {
		_, _ = d.ListDatabases(ctx, nil, false)
	}
}
