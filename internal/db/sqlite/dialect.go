// Package sqlite is the SQLite implementation of db.Dialect.
package sqlite

import (
	_ "github.com/mattn/go-sqlite3"

	"github.com/conray/dataseai/internal/db"
)

// SQLite implements db.Dialect for SQLite. All methods are spread across
// this package, grouped by concern (dsn.go, ident.go, schema.go, browse.go,
// dml.go, kill.go, sqlclass.go). The package init registers the singleton.
type SQLite struct {
	db.UnimplementedDialect
}

func (SQLite) Engine() db.Engine  { return db.EngineSQLite }
func (SQLite) DriverName() string { return "sqlite3" }

var singleton = SQLite{}

func init() { db.Register(db.EngineSQLite, singleton) }
