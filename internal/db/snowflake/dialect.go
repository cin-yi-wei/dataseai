// Package snowflake is the Snowflake implementation of db.Dialect.
package snowflake

import (
	_ "github.com/snowflakedb/gosnowflake"

	"github.com/conray/dataseai/internal/db"
)

// Snowflake implements db.Dialect for Snowflake.
type Snowflake struct {
	db.UnimplementedDialect
}

func (Snowflake) Engine() db.Engine  { return db.EngineSnowflake }
func (Snowflake) DriverName() string { return "snowflake" }

var singleton = Snowflake{}

func init() { db.Register(db.EngineSnowflake, singleton) }
