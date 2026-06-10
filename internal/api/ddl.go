package api

import (
	"context"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/db"
	"github.com/go-chi/chi/v5"
)

// handleTruncateTable executes TRUNCATE TABLE for the requested table.
// Wired to POST /api/db/{connId}/databases/{db}/tables/{table}/truncate.
// Enforces the same DML write policy as a delete-all-rows would.
func handleTruncateTable(d Deps) http.HandlerFunc {
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
		if !enforceDMLPolicy(d, w, r, cs, schema, table, db.OpDelete) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		sql := "TRUNCATE TABLE " + cs.Dialect.QuoteIdent(schema) + "." + cs.Dialect.QuoteIdent(table)
		_, err := cs.DB.ExecContext(ctx, sql)
		recordDMLAudit(d, r, cs, schema, table, db.OpDelete, sql, 0, err)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// handleDropTable executes DROP TABLE for the requested table.
// Wired to DELETE /api/db/{connId}/databases/{db}/tables/{table}.
// Enforces the DML write policy as a delete-all-rows would; the client
// is expected to gate this behind an explicit confirmation prompt.
func handleDropTable(d Deps) http.HandlerFunc {
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
		if !enforceDMLPolicy(d, w, r, cs, schema, table, db.OpDelete) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		sql := "DROP TABLE " + cs.Dialect.QuoteIdent(schema) + "." + cs.Dialect.QuoteIdent(table)
		_, err := cs.DB.ExecContext(ctx, sql)
		recordDMLAudit(d, r, cs, schema, table, db.OpDelete, sql, 0, err)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
