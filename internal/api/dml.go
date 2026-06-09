package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/db"
	"github.com/conray/dataseai/internal/policy"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

// enforceDMLPolicy gates a row-mutation handler behind the user's DataGrid
// write policy. Returns true if the request can proceed; false means the
// response has already been written.
func enforceDMLPolicy(d Deps, w http.ResponseWriter, r *http.Request, cs *connSession, dbName, table string, op db.Op) bool {
	u, _ := auth.UserFromContext(r.Context())
	dec := policy.Check(d.Store, u.ID, cs.Conn.ID, dbName, table, op, store.ScopeDML)
	if dec.Allowed {
		return true
	}
	// Record the blocked attempt so it shows up in the DataGrid audit log.
	_, _ = d.Store.WriteAIAudit(store.AIAuditRow{
		UserID: u.ID, ConnectionID: cs.Conn.ID,
		Database: dbName, Table: table, Operation: string(op),
		Status: "denied", Scope: string(store.ScopeDML),
		ErrorMessage: dec.Reason,
	})
	hint := "請在「設定 → DataGrid 寫入權限」開啟此表的對應操作"
	if dec.Reason == "master_disabled" {
		hint = "請先在「設定 → DataGrid 寫入權限」啟用總開關"
	}
	writeJSON(w, http.StatusForbidden, map[string]any{
		"error":     "policy_denied",
		"reason":    dec.Reason,
		"database":  dbName,
		"table":     table,
		"operation": string(op),
		"hint":      hint,
	})
	return false
}

// recordDMLAudit writes an audit row for an executed DataGrid operation
// (or its failure). Called by the handlers after the mutation completes.
func recordDMLAudit(d Deps, r *http.Request, cs *connSession, dbName, table string, op db.Op, sqlText string, rowsAffected int64, execErr error) {
	u, _ := auth.UserFromContext(r.Context())
	status := "executed"
	errMsg := ""
	var rows *int64
	if execErr != nil {
		status = "failed"
		errMsg = execErr.Error()
	} else {
		rows = &rowsAffected
	}
	_, _ = d.Store.WriteAIAudit(store.AIAuditRow{
		UserID: u.ID, ConnectionID: cs.Conn.ID,
		Database: dbName, Table: table, Operation: string(op),
		SQL: sqlText, Status: status, Scope: string(store.ScopeDML),
		RowsAffected: rows, ErrorMessage: errMsg,
	})
}

type patchRowReq struct {
	PKValues map[string]any `json:"pk_values"`
	Column   string         `json:"column"`
	NewValue any            `json:"new_value"`
}

type insertRowReq struct {
	Values map[string]any `json:"values"`
}

type deleteRowReq struct {
	PKValues map[string]any `json:"pk_values"`
}

func pkOrdered(pkCols []string, values map[string]any) ([]any, bool) {
	out := make([]any, len(pkCols))
	for i, col := range pkCols {
		v, ok := values[col]
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func handlePatchRow(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if schema == "" || table == "" {
			writeError(w, http.StatusBadRequest, "missing db/table")
			return
		}
		var req patchRowReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Column == "" {
			writeError(w, http.StatusBadRequest, "column required")
			return
		}
		if !enforceDMLPolicy(d, w, r, cs, schema, table, db.OpUpdate) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			pkCols, err := primaryKeyViaExecutor(ctx, exec, schema, table)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "pk lookup failed")
				return
			}
			if len(pkCols) == 0 {
				writeError(w, http.StatusUnprocessableEntity, db.ErrNoPrimaryKey.Error())
				return
			}
			pkVals, ok := pkOrdered(pkCols, req.PKValues)
			if !ok {
				writeError(w, http.StatusBadRequest, "pk_values missing required columns")
				return
			}
			n, err := updateCellViaExecutor(ctx, exec, schema, table, pkCols, pkVals, req.Column, req.NewValue)
			recordDMLAudit(d, r, cs, schema, table, db.OpUpdate,
				"UPDATE "+schema+"."+table+" SET "+req.Column+"=? WHERE <pk>", n, err)
			if err != nil {
				if errors.Is(err, db.ErrNoPrimaryKey) {
					writeError(w, http.StatusUnprocessableEntity, err.Error())
					return
				}
				log.Printf("update %s.%s failed: %v", schema, table, err)
				writeError(w, http.StatusInternalServerError, "update failed: "+err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"affected": n})
			return
		}
		pkCols, err := cs.Dialect.PrimaryKey(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "pk lookup failed")
			return
		}
		if len(pkCols) == 0 {
			writeError(w, http.StatusUnprocessableEntity, db.ErrNoPrimaryKey.Error())
			return
		}
		pkVals, ok := pkOrdered(pkCols, req.PKValues)
		if !ok {
			writeError(w, http.StatusBadRequest, "pk_values missing required columns")
			return
		}
		n, err := cs.Dialect.UpdateCell(ctx, cs.DB, schema, table, pkCols, pkVals, req.Column, req.NewValue)
		recordDMLAudit(d, r, cs, schema, table, db.OpUpdate,
			"UPDATE "+schema+"."+table+" SET "+req.Column+"=? WHERE <pk>", n, err)
		if err != nil {
			if errors.Is(err, db.ErrNoPrimaryKey) {
				writeError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			log.Printf("update %s.%s failed: %v", schema, table, err)
			writeError(w, http.StatusInternalServerError, "update failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"affected": n})
	}
}

func handleInsertRow(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if schema == "" || table == "" {
			writeError(w, http.StatusBadRequest, "missing db/table")
			return
		}
		var req insertRowReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if len(req.Values) == 0 {
			writeError(w, http.StatusBadRequest, "values required")
			return
		}
		if !enforceDMLPolicy(d, w, r, cs, schema, table, db.OpInsert) {
			return
		}
		cols := make([]string, 0, len(req.Values))
		vals := make([]any, 0, len(req.Values))
		for col, val := range req.Values {
			cols = append(cols, col)
			vals = append(vals, val)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			id, affected, err := insertRowViaExecutor(ctx, exec, schema, table, req.Values)
			recordDMLAudit(d, r, cs, schema, table, db.OpInsert,
				"INSERT INTO "+schema+"."+table, affected, err)
			if err != nil {
				log.Printf("insert %s.%s failed: %v", schema, table, err)
				writeError(w, http.StatusInternalServerError, "insert failed: "+err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": id})
			return
		}
		id, err := cs.Dialect.InsertRow(ctx, cs.DB, schema, table, cols, vals)
		// We don't have a real "rows affected" for INSERT here — count
		// is 1 on success.
		var affected int64
		if err == nil {
			affected = 1
		}
		recordDMLAudit(d, r, cs, schema, table, db.OpInsert,
			"INSERT INTO "+schema+"."+table, affected, err)
		if err != nil {
			log.Printf("insert %s.%s failed: %v", schema, table, err)
			writeError(w, http.StatusInternalServerError, "insert failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	}
}

func handleDeleteRow(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if schema == "" || table == "" {
			writeError(w, http.StatusBadRequest, "missing db/table")
			return
		}
		var req deleteRowReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if !enforceDMLPolicy(d, w, r, cs, schema, table, db.OpDelete) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			pkCols, err := primaryKeyViaExecutor(ctx, exec, schema, table)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "pk lookup failed")
				return
			}
			if len(pkCols) == 0 {
				writeError(w, http.StatusUnprocessableEntity, "table has no primary key, delete disabled")
				return
			}
			pkVals, ok := pkOrdered(pkCols, req.PKValues)
			if !ok {
				writeError(w, http.StatusBadRequest, "pk_values missing required columns")
				return
			}
			n, err := deleteRowViaExecutor(ctx, exec, schema, table, pkCols, pkVals)
			recordDMLAudit(d, r, cs, schema, table, db.OpDelete,
				"DELETE FROM "+schema+"."+table+" WHERE <pk>", n, err)
			if err != nil {
				if errors.Is(err, db.ErrNoPrimaryKey) {
					writeError(w, http.StatusUnprocessableEntity, err.Error())
					return
				}
				log.Printf("delete %s.%s failed: %v", schema, table, err)
				writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"affected": n})
			return
		}
		pkCols, err := cs.Dialect.PrimaryKey(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "pk lookup failed")
			return
		}
		if len(pkCols) == 0 {
			writeError(w, http.StatusUnprocessableEntity, "table has no primary key, delete disabled")
			return
		}
		pkVals, ok := pkOrdered(pkCols, req.PKValues)
		if !ok {
			writeError(w, http.StatusBadRequest, "pk_values missing required columns")
			return
		}
		n, err := cs.Dialect.DeleteRow(ctx, cs.DB, schema, table, pkCols, pkVals)
		recordDMLAudit(d, r, cs, schema, table, db.OpDelete,
			"DELETE FROM "+schema+"."+table+" WHERE <pk>", n, err)
		if err != nil {
			if errors.Is(err, db.ErrNoPrimaryKey) {
				writeError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			log.Printf("delete %s.%s failed: %v", schema, table, err)
			writeError(w, http.StatusInternalServerError, "delete failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"affected": n})
	}
}
