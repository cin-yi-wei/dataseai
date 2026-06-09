// Package duckdb is the DuckDB implementation of db.Dialect.
// DuckDB is an embedded columnar database that supports SQLite-compatible
// PRAGMA statements and standard information_schema queries.
package duckdb

import (
	_ "github.com/marcboeker/go-duckdb"

	"github.com/conray/dataseai/internal/db"
)

// DuckDB implements db.Dialect for DuckDB.
type DuckDB struct {
	db.UnimplementedDialect
}

func (DuckDB) Engine() db.Engine  { return db.EngineDuckDB }
func (DuckDB) DriverName() string { return "duckdb" }

var singleton = DuckDB{}

func init() { db.Register(db.EngineDuckDB, singleton) }
