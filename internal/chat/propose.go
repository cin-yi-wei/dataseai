package chat

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/policy"
	"github.com/conray/dataseai/internal/store"
)

// ExecCtx carries the per-session context that propose_write (and any future
// write-path tools) need beyond just a db handle.
type ExecCtx struct {
	DB        *sql.DB
	Store     *store.Store
	Gateway   ProposalGateway
	UserID    int64
	ConnID    int64
	DefaultDB string
}

func proposalID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func jsonObj(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// handleProposeWrite is invoked from Execute when the LLM calls propose_write.
// Returns a JSON string (the tool_result body). Never returns a non-nil error
// for user-visible failures — those are encoded in the JSON instead, so the
// LLM can handle them gracefully.
func handleProposeWrite(ctx context.Context, ec ExecCtx, input map[string]any) (string, error) {
	// 1. Parse and validate declared fields.
	var decl struct {
		Database, Table, Operation, SQL string
	}
	decl.Database, _ = input["database"].(string)
	decl.Table, _ = input["table"].(string)
	decl.Operation, _ = input["operation"].(string)
	decl.SQL, _ = input["sql"].(string)
	if decl.Database == "" || decl.Table == "" || decl.Operation == "" || decl.SQL == "" {
		return jsonObj(map[string]any{"error": "invalid_proposal", "reason": "missing field"}), nil
	}

	// 2. Classify the SQL.
	cls, err := mysql.ClassifySQL(decl.SQL)
	if err != nil {
		return jsonObj(map[string]any{"error": "invalid_proposal", "reason": err.Error()}), nil
	}
	if cls.Multi {
		return jsonObj(map[string]any{"error": "multi_statement", "reason": "one statement at a time"}), nil
	}

	// 3. Verify declared operation matches classified operation.
	declOp := opFromDecl(decl.Operation)
	if !classifiedMatches(cls, declOp) {
		return jsonObj(map[string]any{
			"error":  "invalid_proposal",
			"reason": fmt.Sprintf("classified as %s, declared %s", cls.Op, declOp),
		}), nil
	}

	// 4. Resolve db/table from classifier (falls back to declared, then DefaultDB).
	db := cls.DB
	if db == "" {
		db = decl.Database
	}
	if db == "" {
		db = ec.DefaultDB
	}
	if cls.Table == "" || db == "" {
		return jsonObj(map[string]any{"error": "invalid_proposal", "reason": "could not resolve db/table"}), nil
	}

	// 5. Verify declared db/table match what the SQL actually targets.
	if !ciEq(db, decl.Database) || !ciEq(cls.Table, decl.Table) {
		return jsonObj(map[string]any{
			"error":  "invalid_proposal",
			"reason": fmt.Sprintf("sql targets %s.%s, declared %s.%s", db, cls.Table, decl.Database, decl.Table),
		}), nil
	}

	// 6. Policy check (pre-execute).
	dec := policy.Check(ec.Store, ec.UserID, ec.ConnID, decl.Database, decl.Table, declOp)
	if !dec.Allowed {
		_, _ = ec.Store.WriteAIAudit(store.AIAuditRow{
			UserID: ec.UserID, ConnectionID: ec.ConnID,
			Database: decl.Database, Table: decl.Table, Operation: string(declOp),
			SQL: decl.SQL, Status: "denied", ErrorMessage: dec.Reason,
		})
		return jsonObj(map[string]any{
			"error":     "policy_denied",
			"reason":    dec.Reason,
			"database":  decl.Database,
			"table":     decl.Table,
			"operation": string(declOp),
			"hint":      "ask the user to enable this in Settings → AI 寫入權限",
		}), nil
	}

	// 7. Optionally run EXPLAIN for UPDATE/DELETE to inform the user.
	explainJSON := ""
	if declOp == mysql.OpUpdate || declOp == mysql.OpDelete {
		explainJSON = runExplain(ctx, ec.DB, decl.SQL)
	}

	// 8. Write initial audit row (status=proposed).
	audID, err := ec.Store.WriteAIAudit(store.AIAuditRow{
		UserID: ec.UserID, ConnectionID: ec.ConnID,
		Database: decl.Database, Table: decl.Table, Operation: string(declOp),
		SQL: decl.SQL, Status: "proposed", ExplainSummary: explainJSON,
	})
	if err != nil {
		return jsonObj(map[string]any{"error": "audit_unavailable", "reason": err.Error()}), nil
	}

	// 9. No gateway — cancel immediately.
	if ec.Gateway == nil {
		_ = ec.Store.UpdateAIAuditStatus(audID, "cancelled", nil, "no gateway")
		return jsonObj(map[string]any{"status": "cancelled", "error": "no gateway configured"}), nil
	}

	// 10. Send proposal to the WS layer and block for user decision.
	d, err := ec.Gateway.Propose(ctx, Proposal{
		ID:             proposalID(),
		Database:       decl.Database,
		Table:          decl.Table,
		Operation:      string(declOp),
		SQL:            decl.SQL,
		ExplainSummary: explainJSON,
	})
	if err != nil {
		_ = ec.Store.UpdateAIAuditStatus(audID, "cancelled", nil, err.Error())
		return jsonObj(map[string]any{"status": "cancelled", "error": err.Error()}), nil
	}
	if !d.Accept {
		_ = ec.Store.UpdateAIAuditStatus(audID, "cancelled", nil, "")
		return jsonObj(map[string]any{"status": "cancelled"}), nil
	}

	// 11. Re-check policy at execute time (may have been revoked while waiting).
	dec2 := policy.Check(ec.Store, ec.UserID, ec.ConnID, decl.Database, decl.Table, declOp)
	if !dec2.Allowed {
		_ = ec.Store.UpdateAIAuditStatus(audID, "denied", nil, dec2.Reason)
		return jsonObj(map[string]any{"error": "policy_denied", "reason": "revoked before execute"}), nil
	}

	// 12. Execute.
	res, err := ec.DB.ExecContext(ctx, decl.SQL)
	if err != nil {
		_ = ec.Store.UpdateAIAuditStatus(audID, "failed", nil, err.Error())
		return jsonObj(map[string]any{"status": "failed", "error": err.Error()}), nil
	}
	n, _ := res.RowsAffected()
	_ = ec.Store.UpdateAIAuditStatus(audID, "executed", &n, "")
	return jsonObj(map[string]any{"status": "executed", "rows_affected": n}), nil
}

func opFromDecl(s string) mysql.Op {
	switch s {
	case "INSERT":
		return mysql.OpInsert
	case "UPDATE":
		return mysql.OpUpdate
	case "DELETE":
		return mysql.OpDelete
	case "TRUNCATE":
		return mysql.OpTruncate
	case "DDL":
		return mysql.OpDDL
	}
	return mysql.OpUnknown
}

func classifiedMatches(cls mysql.Classified, declOp mysql.Op) bool {
	switch declOp {
	case mysql.OpInsert, mysql.OpUpdate, mysql.OpDelete, mysql.OpTruncate, mysql.OpDDL:
		return cls.Op == declOp
	}
	return false
}

// ciEq compares two strings case-insensitively (suitable for db/table names).
func ciEq(a, b string) bool { return strings.EqualFold(a, b) }

// runExplain runs EXPLAIN on the given SQL and returns the result as a JSON string.
// Used for UPDATE/DELETE to show the user the affected rows estimate.
func runExplain(ctx context.Context, db *sql.DB, sqlText string) string {
	if db == nil {
		return ""
	}
	rows, err := db.QueryContext(ctx, "EXPLAIN "+sqlText)
	if err != nil {
		return jsonObj(map[string]any{"error": err.Error()})
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var data []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return jsonObj(map[string]any{"error": err.Error()})
		}
		row := map[string]any{}
		for i, c := range cols {
			row[c] = vals[i]
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return jsonObj(map[string]any{"error": err.Error()})
	}
	return jsonObj(map[string]any{"rows": data})
}
