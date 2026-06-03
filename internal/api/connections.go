package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/mysql"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

type connectionReq struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	DefaultDB string `json:"default_db,omitempty"`
	TLS       string `json:"tls,omitempty"`
	Color     string `json:"color,omitempty"`
}

func (r connectionReq) validate() error {
	if r.Name == "" || len(r.Name) > 64 {
		return errors.New("name required (1-64 chars)")
	}
	if r.Host == "" {
		return errors.New("host required")
	}
	if r.Username == "" {
		return errors.New("username required")
	}
	if r.TLS != "" && r.TLS != "disabled" && r.TLS != "preferred" && r.TLS != "required" {
		return errors.New("tls must be disabled|preferred|required")
	}
	return nil
}

func connectionJSON(c store.Connection) map[string]any {
	return map[string]any{
		"id":         c.ID,
		"name":       c.Name,
		"host":       c.Host,
		"port":       c.Port,
		"username":   c.Username,
		"default_db": c.DefaultDB,
		"tls":        c.TLS,
		"color":      c.Color,
		"created_at": c.CreatedAt,
		"updated_at": c.UpdatedAt,
	}
}

func handleCreateConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var req connectionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := req.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		c, err := d.Store.CreateConnection(d.Cipher, u.ID, store.ConnectionInput{
			Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
			DefaultDB: req.DefaultDB, TLS: req.TLS, Color: req.Color,
		})
		if err != nil {
			if errors.Is(err, store.ErrDuplicate) {
				writeError(w, http.StatusConflict, "name already used")
				return
			}
			writeError(w, http.StatusInternalServerError, "create connection failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection": connectionJSON(c)})
	}
}

func handleListConnections(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		list, err := d.Store.ListConnections(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(list))
		for _, c := range list {
			out = append(out, connectionJSON(c))
		}
		writeJSON(w, http.StatusOK, map[string]any{"connections": out})
	}
}

func parseConnIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func handleGetConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := parseConnIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		c, err := d.Store.GetConnection(u.ID, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "get failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"connection": connectionJSON(c)})
	}
}

func handleUpdateConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := parseConnIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		var req connectionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := req.validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		c, err := d.Store.UpdateConnection(d.Cipher, u.ID, id, store.ConnectionInput{
			Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
			DefaultDB: req.DefaultDB, TLS: req.TLS, Color: req.Color,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			if errors.Is(err, store.ErrDuplicate) {
				writeError(w, http.StatusConflict, "name already used")
				return
			}
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		d.Pool.Evict(mysql.PoolKey{UserID: u.ID, ConnID: id})
		writeJSON(w, http.StatusOK, map[string]any{"connection": connectionJSON(c)})
	}
}

func handleDeleteConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := parseConnIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		if err := d.Store.DeleteConnection(u.ID, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		d.Pool.Evict(mysql.PoolKey{UserID: u.ID, ConnID: id})
		w.WriteHeader(http.StatusNoContent)
	}
}

func contextWithTimeout(parent context.Context, seconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, time.Duration(seconds)*time.Second)
}

func handleTestConnection(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := parseConnIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		conn, err := d.Store.GetConnection(u.ID, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "decrypt failed")
			return
		}
		dsn := mysql.BuildDSN(mysql.DSNInput{
			Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
			DefaultDB: conn.DefaultDB, TLS: conn.TLS,
		})
		db, err := d.Pool.Get(mysql.PoolKey{UserID: u.ID, ConnID: id}, dsn)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		ctx, cancel := contextWithTimeout(r.Context(), 5)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			d.Pool.Evict(mysql.PoolKey{UserID: u.ID, ConnID: id})
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "connected"})
	}
}
