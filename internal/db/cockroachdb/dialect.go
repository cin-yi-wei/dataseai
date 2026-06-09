// Package cockroachdb is the CockroachDB implementation of db.Dialect.
// CockroachDB speaks the PostgreSQL wire protocol; this package wraps the
// PostgreSQL dialect and overrides only the Engine name. All DSN building,
// SSH tunneling, schema introspection, and DML use the pg implementations.
package cockroachdb

import (
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/conray/dataseai/internal/db"
	pgdialect "github.com/conray/dataseai/internal/db/pg"
)

// CockroachDB embeds PG to inherit all dialect methods. Only Engine() is
// overridden so the registry and UI treat it as a distinct engine.
type CockroachDB struct {
	pgdialect.PG
}

func (CockroachDB) Engine() db.Engine { return db.EngineCockroachDB }

var singleton = CockroachDB{}

func init() { db.Register(db.EngineCockroachDB, singleton) }
