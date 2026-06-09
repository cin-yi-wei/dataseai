// Package clickhouse is the ClickHouse implementation of db.Dialect.
package clickhouse

import (
	_ "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/conray/dataseai/internal/db"
)

// CH implements db.Dialect for ClickHouse.
type CH struct {
	db.UnimplementedDialect
}

func (CH) Engine() db.Engine  { return db.EngineClickHouse }
func (CH) DriverName() string { return "clickhouse" }

var singleton = CH{}

func init() { db.Register(db.EngineClickHouse, singleton) }
