// Package oracle is the Oracle Database implementation of db.Dialect.
// Uses go-ora/v2 (pure Go, no Oracle Instant Client required).
package oracle

import (
	_ "github.com/sijms/go-ora/v2"

	"github.com/conray/dataseai/internal/db"
)

// Oracle implements db.Dialect for Oracle Database.
type Oracle struct {
	db.UnimplementedDialect
}

func (Oracle) Engine() db.Engine  { return db.EngineOracle }
func (Oracle) DriverName() string { return "oracle" }

var singleton = Oracle{}

func init() { db.Register(db.EngineOracle, singleton) }
