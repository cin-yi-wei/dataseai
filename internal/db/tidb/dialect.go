// Package tidb is the TiDB implementation of db.Dialect.
// TiDB is wire-compatible with MySQL; this package wraps the MySQL dialect
// and overrides only the Engine name. All DSN building, SSH tunneling, schema
// introspection, and DML use the MySQL implementations unchanged.
package tidb

import (
	_ "github.com/go-sql-driver/mysql"

	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
)

// TiDB embeds MySQL to inherit all dialect methods. Only Engine() is
// overridden so the registry and UI treat it as a distinct engine.
type TiDB struct {
	mysqldialect.MySQL
}

func (TiDB) Engine() db.Engine { return db.EngineTiDB }

var singleton = TiDB{}

func init() { db.Register(db.EngineTiDB, singleton) }
