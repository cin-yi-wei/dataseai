// Package bytehouse is the ByteHouse (ClickHouse-compatible) implementation
// of db.Dialect. ByteHouse is a cloud-native analytical database developed
// by ByteDance, protocol-compatible with ClickHouse.
package bytehouse

import (
	"github.com/conray/dataseai/internal/db"

	_ "github.com/bytehouse-cloud/driver-go/sql"
)

// BH is the ByteHouse dialect. Methods split across dsn.go / ident.go /
// schema.go / browse.go / dml.go / kill.go / sqlclass.go / query.go.
type BH struct {
	db.UnimplementedDialect
}

func (BH) Engine() db.Engine  { return db.EngineBytehouse }
func (BH) DriverName() string { return "bytehouse" }

var singleton = BH{}

func init() {
	db.Register(db.EngineBytehouse, singleton)
}
