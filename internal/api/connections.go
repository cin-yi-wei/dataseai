package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/store"
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
