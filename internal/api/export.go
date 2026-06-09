package api

import (
	"context"
	"net/http"
	"time"

	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	"github.com/go-chi/chi/v5"
)

func handleExport(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		format := r.URL.Query().Get("format")
		if format != "csv" && format != "sql" {
			writeError(w, http.StatusBadRequest, "format must be csv or sql")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		switch format {
		case "csv":
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="`+table+`.csv"`)
			if err := mysqldialect.ExportCSV(ctx, cs.DB, w, schema, table); err != nil {
				_, _ = w.Write([]byte("\n-- export error: " + err.Error() + "\n"))
			}
		case "sql":
			w.Header().Set("Content-Type", "application/sql; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="`+table+`.sql"`)
			if err := mysqldialect.ExportSQL(ctx, cs.DB, w, schema, table); err != nil {
				_, _ = w.Write([]byte("\n-- export error: " + err.Error() + "\n"))
			}
		}
	}
}
