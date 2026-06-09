// Package singlestore is the SingleStore (formerly MemSQL) implementation of
// db.Dialect. SingleStore is wire-compatible with MySQL; this package wraps
// the MySQL dialect and overrides only the Engine name.
package singlestore

import (
	_ "github.com/go-sql-driver/mysql"

	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
)

// SingleStore embeds MySQL to inherit all dialect methods. Only Engine() is
// overridden so the registry and UI treat it as a distinct engine.
type SingleStore struct {
	mysqldialect.MySQL
}

func (SingleStore) Engine() db.Engine { return db.EngineSingleStore }

var singleton = SingleStore{}

func init() { db.Register(db.EngineSingleStore, singleton) }
