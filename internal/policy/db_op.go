package policy

import (
	"github.com/conray/dataseai/internal/db"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
)

// CheckDBOp is an additive bridge over Check that accepts an engine-agnostic
// db.Op. It exists so callers already migrated to internal/db (e.g. chat)
// don't have to drag internal/mysql back in just to satisfy the legacy
// policy.Check signature. Once internal/policy is migrated off mysql.Op in
// a later task, this shim can be deleted and Check itself will take db.Op.
func CheckDBOp(s *store.Store, userID, connID int64, dbName, table string, op db.Op, scope store.PolicyScope) Decision {
	return Check(s, userID, connID, dbName, table, mysql.Op(op), scope)
}
