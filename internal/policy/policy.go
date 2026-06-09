package policy

import (
	"github.com/conray/dataseai/internal/db"
	"github.com/conray/dataseai/internal/store"
)

// Decision is the result of a policy check.
type Decision struct {
	Allowed bool
	Reason  string // master_disabled | policy_denied | ""
}

// Check applies the two-layer gate for a given scope (ai | dml):
//  1. master switch (ai_writes_enabled or dml_writes_enabled)
//  2. per-(user, conn, db, table, scope, op) policy row
func Check(s *store.Store, userID, connID int64, dbName, table string, op db.Op, scope store.PolicyScope) Decision {
	enabled, err := s.GetWritesEnabled(userID, scope)
	if err != nil || !enabled {
		return Decision{false, "master_disabled"}
	}
	p, found, err := s.GetWritePolicy(userID, connID, dbName, table, scope)
	if err != nil || !found {
		return Decision{false, "policy_denied"}
	}
	switch op {
	case db.OpInsert:
		if p.Insert {
			return Decision{true, ""}
		}
	case db.OpUpdate:
		if p.Update {
			return Decision{true, ""}
		}
	case db.OpDelete, db.OpTruncate:
		if p.Delete {
			return Decision{true, ""}
		}
	case db.OpDDL:
		if p.DDL {
			return Decision{true, ""}
		}
	}
	return Decision{false, "policy_denied"}
}
