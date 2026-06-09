// Package mssql is the Microsoft SQL Server implementation of db.Dialect.
package mssql

import (
	"github.com/conray/dataseai/internal/db"

	_ "github.com/microsoft/go-mssqldb"
)

// MSSQL is the SQL Server dialect. Methods split across dsn.go / ident.go /
// schema.go / browse.go / dml.go / kill.go / sqlclass.go / query.go.
// Package init registers the singleton and the SSH wrapper driver.
type MSSQL struct {
	db.UnimplementedDialect
}

func (MSSQL) Engine() db.Engine  { return db.EngineMSSQL }
func (MSSQL) DriverName() string { return driverName }

var singleton = MSSQL{}

func init() {
	registerSSHDriver()
	db.Register(db.EngineMSSQL, singleton)
}
