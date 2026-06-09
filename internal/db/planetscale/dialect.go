// Package planetscale is the PlanetScale implementation of db.Dialect.
// PlanetScale is wire-compatible with MySQL; this package wraps the MySQL
// dialect and overrides only the Engine name.
package planetscale

import (
	_ "github.com/go-sql-driver/mysql"

	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
)

// PlanetScale embeds MySQL to inherit all dialect methods.
type PlanetScale struct {
	mysqldialect.MySQL
}

func (PlanetScale) Engine() db.Engine { return db.EnginePlanetScale }

var singleton = PlanetScale{}

func init() { db.Register(db.EnginePlanetScale, singleton) }
