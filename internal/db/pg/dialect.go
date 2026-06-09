// Package pg is the PostgreSQL implementation of db.Dialect.
package pg

import (
	"github.com/conray/dataseai/internal/db"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PG is the PostgreSQL dialect. Methods are split across dsn.go / ident.go /
// schema.go / browse.go / dml.go / kill.go / sqlclass.go. Package init
// registers the singleton.
type PG struct {
	db.UnimplementedDialect
}

func (PG) Engine() db.Engine  { return db.EnginePostgres }
func (PG) DriverName() string { return "pgx" }

var singleton = PG{}

func init() {
	db.Register(db.EnginePostgres, singleton)
}
