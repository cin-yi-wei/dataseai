package chat

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	"github.com/conray/dataseai/internal/policy"
	"github.com/conray/dataseai/internal/store"
)

// ExecCtx carries the per-session context that propose_write (and any future
// write-path tools) need beyond just a db handle.
type ExecCtx struct {
	DB        *sql.DB
	Executor  mysqldialect.Executor
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

func (ec ExecCtx) executor() (mysqldialect.Executor, error) {
	if ec.Executor != nil {
		return ec.Executor, nil
	}
	if ec.DB != nil {
		return mysqldialect.DirectExecutor{DB: ec.DB}, nil
	}
	return nil, fmt.Errorf("database executor unavailable")
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
	cls, err := mysqldialect.MySQL{}.ClassifySQL(decl.SQL)
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
	dbName := cls.DB
	if dbName == "" {
		dbName = decl.Database
	}
	if dbName == "" {
		dbName = ec.DefaultDB
	}
	if cls.Table == "" || dbName == "" {
		return jsonObj(map[string]any{"error": "invalid_proposal", "reason": "could not resolve db/table"}), nil
	}

	// 5. Verify declared db/table match what the SQL actually targets.
	if !ciEq(dbName, decl.Database) || !ciEq(cls.Table, decl.Table) {
		return jsonObj(map[string]any{
			"error":  "invalid_proposal",
			"reason": fmt.Sprintf("sql targets %s.%s, declared %s.%s", dbName, cls.Table, decl.Database, decl.Table),
		}), nil
	}

	// 5b. Session-scope: when the chat is pinned to a database, refuse any
	// write proposal aimed at a different one. Without this the policy check
	// would still deny (default-deny per-table), but a clear scope error is
	// more useful than "policy_denied" for the LLM.
	if ec.DefaultDB != "" && !ciEq(dbName, ec.DefaultDB) {
		return jsonObj(map[string]any{
			"error":        "db_scope_denied",
			"reason":       "this chat session is pinned to a single database; writes must target it",
			"requested_db": dbName,
		}), nil
	}

	// 6. Policy check (pre-execute).
	dec := policy.Check(ec.Store, ec.UserID, ec.ConnID, decl.Database, decl.Table, declOp, store.ScopeAI)
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
	if declOp == db.OpUpdate || declOp == db.OpDelete {
		exec, err := ec.executor()
		if err == nil {
			explainJSON = runExplain(ctx, exec, decl.SQL)
		}
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
	dec2 := policy.Check(ec.Store, ec.UserID, ec.ConnID, decl.Database, decl.Table, declOp, store.ScopeAI)
	if !dec2.Allowed {
		_ = ec.Store.UpdateAIAuditStatus(audID, "denied", nil, dec2.Reason)
		return jsonObj(map[string]any{"error": "policy_denied", "reason": "revoked before execute"}), nil
	}

	// 12. Execute.
	exec, err := ec.executor()
	if err != nil {
		_ = ec.Store.UpdateAIAuditStatus(audID, "failed", nil, err.Error())
		return jsonObj(map[string]any{"status": "failed", "error": err.Error()}), nil
	}
	res, err := exec.Run(ctx, decl.SQL, mysqldialect.RunOpts{})
	if err != nil {
		_ = ec.Store.UpdateAIAuditStatus(audID, "failed", nil, err.Error())
		return jsonObj(map[string]any{"status": "failed", "error": err.Error()}), nil
	}
	n := res.RowsAffected
	_ = ec.Store.UpdateAIAuditStatus(audID, "executed", &n, "")
	return jsonObj(map[string]any{"status": "executed", "rows_affected": n}), nil
}

func opFromDecl(s string) db.Op {
	switch s {
	case "INSERT":
		return db.OpInsert
	case "UPDATE":
		return db.OpUpdate
	case "DELETE":
		return db.OpDelete
	case "TRUNCATE":
		return db.OpTruncate
	case "DDL":
		return db.OpDDL
	}
	return db.OpUnknown
}

func classifiedMatches(cls db.Classified, declOp db.Op) bool {
	switch declOp {
	case db.OpInsert, db.OpUpdate, db.OpDelete, db.OpTruncate, db.OpDDL:
		return cls.Op == declOp
	}
	return false
}

// ciEq compares two strings case-insensitively (suitable for db/table names).
func ciEq(a, b string) bool { return strings.EqualFold(a, b) }

// runExplain runs EXPLAIN on the given SQL and returns the result as a JSON string.
// Used for UPDATE/DELETE to show the user the affected rows estimate.
func runExplain(ctx context.Context, exec mysqldialect.Executor, sqlText string) string {
	out, err := exec.Run(ctx, "EXPLAIN "+sqlText, mysqldialect.RunOpts{})
	if err != nil {
		return jsonObj(map[string]any{"error": err.Error()})
	}
	var data []map[string]any
	for _, vals := range out.Rows {
		row := map[string]any{}
		for i, c := range out.Columns {
			if i < len(vals) {
				row[c] = vals[i]
			}
		}
		data = append(data, row)
	}
	return jsonObj(map[string]any{"rows": data})
}
