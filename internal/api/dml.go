package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/mysql"
	"github.com/go-chi/chi/v5"
)

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
		cols := make([]string, 0, len(req.Values))
		vals := make([]any, 0, len(req.Values))
		for col, val := range req.Values {
			cols = append(cols, col)
			vals = append(vals, val)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		id, err := mysql.InsertRow(ctx, cs.DB, schema, table, cols, vals)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "insert failed")
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		pkCols, err := mysql.PrimaryKey(ctx, cs.DB, schema, table)
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
		n, err := mysql.DeleteRow(ctx, cs.DB, schema, table, pkCols, pkVals)
		if err != nil {
			if errors.Is(err, mysql.ErrNoPrimaryKey) {
				writeError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"affected": n})
	}
}
