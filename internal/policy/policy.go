package policy

import (
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
)

// Decision is the result of a policy check.
type Decision struct {
	Allowed bool
	Reason  string // master_disabled | policy_denied | ""
}

// Check applies the two-layer gate:
//  1. master switch (ai_writes_enabled on the user)
//  2. per-(user, conn, db, table, op) policy row
func Check(s *store.Store, userID, connID int64, db, table string, op mysql.Op) Decision {
	enabled, err := s.GetAIWritesEnabled(userID)
	if err != nil || !enabled {
		return Decision{false, "master_disabled"}
	}
	p, found, err := s.GetAIPolicy(userID, connID, db, table)
	if err != nil || !found {
		return Decision{false, "policy_denied"}
	}
	switch op {
	case mysql.OpInsert:
		if p.Insert {
			return Decision{true, ""}
		}
	case mysql.OpUpdate:
		if p.Update {
			return Decision{true, ""}
		}
	case mysql.OpDelete, mysql.OpTruncate:
		if p.Delete {
			return Decision{true, ""}
		}
	case mysql.OpDDL:
		if p.DDL {
			return Decision{true, ""}
		}
	}
	return Decision{false, "policy_denied"}
}
