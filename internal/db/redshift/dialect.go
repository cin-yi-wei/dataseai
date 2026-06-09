// Package redshift is the Amazon Redshift implementation of db.Dialect.
// Redshift speaks the PostgreSQL wire protocol; this package wraps the
// PostgreSQL dialect and overrides only the Engine name.
package redshift

import (
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/conray/dataseai/internal/db"
	pgdialect "github.com/conray/dataseai/internal/db/pg"
)

// Redshift embeds PG to inherit all dialect methods. Only Engine() is
// overridden so the registry and UI treat it as a distinct engine.
type Redshift struct {
	pgdialect.PG
}

func (Redshift) Engine() db.Engine { return db.EngineRedshift }

var singleton = Redshift{}

func init() { db.Register(db.EngineRedshift, singleton) }
