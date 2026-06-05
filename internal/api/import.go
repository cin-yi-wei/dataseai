package api

import (
	"context"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/mysql"
	"github.com/go-chi/chi/v5"
)

func handleImport(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "bad multipart")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "no file")
			return
		}
		defer f.Close()
		if !enforceDMLPolicy(d, w, r, cs, schema, table, mysql.OpInsert) {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()
		inserted, errs, err := mysql.ImportCSV(ctx, cs.DB, f, schema, table)
		recordDMLAudit(d, r, cs, schema, table, mysql.OpInsert,
			"IMPORT CSV → "+schema+"."+table, int64(inserted), err)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"rows_inserted": inserted,
			"errors":        errs,
		})
	}
}
