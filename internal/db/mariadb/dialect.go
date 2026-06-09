// Package mariadb is the MariaDB implementation of db.Dialect.
// MariaDB is wire-compatible with MySQL; this package wraps the MySQL dialect
// and overrides only the Engine name. All DSN building, SSH tunneling, schema
// introspection, and DML use the MySQL implementations unchanged.
package mariadb

import (
	_ "github.com/go-sql-driver/mysql"

	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
)

// MariaDB embeds MySQL to inherit all dialect methods. Only Engine() is
// overridden so the registry and UI treat it as a distinct engine.
type MariaDB struct {
	mysqldialect.MySQL
}

func (MariaDB) Engine() db.Engine { return db.EngineMariaDB }

var singleton = MariaDB{}

func init() { db.Register(db.EngineMariaDB, singleton) }
