// Package mysql is the MySQL implementation of db.Dialect.
package mysql

import (
	"github.com/conray/dataseai/internal/db"
)

// MySQL is the MySQL dialect. Its methods live across this package, grouped
// by concern (dsn.go, ident.go, schema.go, browse.go, dml.go, kill.go,
// sqlclass.go). The package init registers the singleton into db's
// registry so consumers obtain it via db.MustGet(db.EngineMySQL).
type MySQL struct {
	db.UnimplementedDialect
}

func (MySQL) Engine() db.Engine  { return db.EngineMySQL }
func (MySQL) DriverName() string { return "mysql" }

var singleton = MySQL{}

func init() {
	db.Register(db.EngineMySQL, singleton)
}
