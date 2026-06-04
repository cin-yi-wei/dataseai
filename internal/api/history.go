package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

func handleListHistory(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		list, err := d.Store.ListHistory(u.ID, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(list))
		for _, h := range list {
			out = append(out, map[string]any{
				"id":            h.ID,
				"connection_id": h.ConnectionID,
				"database_name": h.DatabaseName,
				"sql_text":      h.SQLText,
				"duration_ms":   h.DurationMs,
				"rows_affected": h.RowsAffected,
				"error_message": h.ErrorMessage,
				"source":        h.Source,
				"executed_at":   h.ExecutedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"history": out})
	}
}

func handleDeleteHistoryEntry(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		if err := d.Store.DeleteHistoryEntry(u.ID, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleClearHistory(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		if err := d.Store.ClearHistory(u.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "clear failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
