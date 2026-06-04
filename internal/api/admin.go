package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

func handleAdminStats(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := d.Store.GetAdminStats()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stats)
	}
}

func handleAdminListUsers(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := d.Store.ListUsers()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": users})
	}
}

func handleAdminDeleteUser(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		if id == u.ID {
			writeError(w, http.StatusBadRequest, "cannot delete yourself")
			return
		}
		if err := d.Store.DeleteUser(id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleAdminSetAdmin(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		var body struct {
			IsAdmin bool `json:"is_admin"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		// Prevent removing your own admin if you're the only admin
		if id == u.ID && !body.IsAdmin {
			stats, _ := d.Store.GetAdminStats()
			if stats.TotalAdmins <= 1 {
				writeError(w, http.StatusBadRequest, "cannot remove last admin")
				return
			}
		}
		if err := d.Store.SetUserAdmin(id, body.IsAdmin); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleAdminListConnections(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conns, err := d.Store.ListAllConnections()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connections": conns})
	}
}
