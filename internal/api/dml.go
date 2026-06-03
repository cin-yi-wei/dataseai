package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/conray/mysqlweb/internal/mysql"
	"github.com/go-chi/chi/v5"
)

type patchRowReq struct {
	PKValues map[string]any `json:"pk_values"`
	Column   string         `json:"column"`
	NewValue any            `json:"new_value"`
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		pkCols, err := mysql.PrimaryKey(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "pk lookup failed")
			return
		}
		if len(pkCols) == 0 {
			writeError(w, http.StatusUnprocessableEntity, mysql.ErrNoPrimaryKey.Error())
			return
		}
		pkVals, ok := pkOrdered(pkCols, req.PKValues)
		if !ok {
			writeError(w, http.StatusBadRequest, "pk_values missing required columns")
			return
		}
		n, err := mysql.UpdateCell(ctx, cs.DB, schema, table, pkCols, pkVals, req.Column, req.NewValue)
		if err != nil {
			if errors.Is(err, mysql.ErrNoPrimaryKey) {
				writeError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"affected": n})
	}
}
