package api

import (
	"context"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/db"
	bhdialect "github.com/conray/dataseai/internal/db/bytehouse"
	mssqldialect "github.com/conray/dataseai/internal/db/mssql"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	pgdialect "github.com/conray/dataseai/internal/db/pg"
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
		if !enforceDMLPolicy(d, w, r, cs, schema, table, db.OpInsert) {
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()
		var inserted int
		var errs []string
		format := r.URL.Query().Get("format")
		if format == "sql" {
			inserted, errs, err = importSQL(ctx, cs.DB, f, schema)
			recordDMLAudit(d, r, cs, schema, table, db.OpInsert,
				"IMPORT SQL → "+schema+"."+table, int64(inserted), err)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"statements_executed": inserted,
				"errors":              errs,
			})
			return
		}
		switch cs.Dialect.Engine() {
		case db.EnginePostgres:
			inserted, errs, err = pgdialect.ImportCSV(ctx, cs.DB, f, schema, table)
		case db.EngineMSSQL:
			inserted, errs, err = mssqldialect.ImportCSV(ctx, cs.DB, f, schema, table)
		case db.EngineBytehouse:
			inserted, errs, err = bhdialect.ImportCSV(ctx, cs.DB, f, schema, table)
		default:
			inserted, errs, err = mysqldialect.ImportCSV(ctx, cs.DB, f, schema, table)
		}
		recordDMLAudit(d, r, cs, schema, table, db.OpInsert,
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
