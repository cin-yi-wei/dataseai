package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/conray/dataseai/internal/agent"
	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

type connectionReq struct {
	ID               *int64 `json:"id,omitempty"`
	Name             string `json:"name"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	DefaultDB        string `json:"default_db,omitempty"`
	TLS              string `json:"tls,omitempty"`
	Color            string `json:"color,omitempty"`
	GroupName        string `json:"group_name,omitempty"`
	Engine           string `json:"engine,omitempty"`
	SSHEnabled       bool   `json:"ssh_enabled,omitempty"`
	SSHHost          string `json:"ssh_host,omitempty"`
	SSHPort          int    `json:"ssh_port,omitempty"`
	SSHUser          string `json:"ssh_user,omitempty"`
	SSHPassword      string `json:"ssh_password,omitempty"`
	SSHKey           string `json:"ssh_key,omitempty"`
	SSHKeyPassphrase string `json:"ssh_key_passphrase,omitempty"`
	ViaAgentID       *int64 `json:"via_agent_id,omitempty"`
}

func (r connectionReq) validate() error {
	if r.Name == "" || len(r.Name) > 64 {
		return errors.New("name required (1-64 chars)")
	}
	return r.validateTarget()
}

func (r connectionReq) validateTarget() error {
	if r.Host == "" {
		return errors.New("host required")
	}
	if r.Username == "" {
		return errors.New("username required")
	}
	if r.TLS != "" && r.TLS != "disabled" && r.TLS != "preferred" && r.TLS != "required" && r.TLS != "skip-verify" {
		return errors.New("tls must be disabled|preferred|required|skip-verify")
	}
	// Engine "" -> default mysql at store layer. Any non-empty value must be
	// a recognized engine (currently only "mysql").
	if r.Engine != "" {
		if _, err := db.ParseEngine(r.Engine); err != nil {
			return errors.New("unsupported engine: " + r.Engine)
		}
	}
	return nil
}

func connectionJSON(c store.Connection) map[string]any {
	engine := c.Engine
	if engine == "" {
		engine = "mysql"
	}
	return map[string]any{
		"id":           c.ID,
		"name":         c.Name,
		"host":         c.Host,
		"port":         c.Port,
		"username":     c.Username,
		"default_db":   c.DefaultDB,
		"tls":          c.TLS,
		"color":        c.Color,
		"group_name":   c.GroupName,
		"engine":       engine,
		"ssh_enabled":  c.SSHEnabled,
		"ssh_host":     c.SSHHost,
		"ssh_port":     c.SSHPort,
		"ssh_user":     c.SSHUser,
		"ssh_key_set":  c.SSHKeySet,
		"via_agent_id": c.ViaAgentID,
		"created_at":   c.CreatedAt,
		"updated_at":   c.UpdatedAt,
	}
}

func validateViaAgent(d Deps, userID int64, agentID *int64) error {
	if agentID == nil {
		return nil
	}
	a, err := d.Store.GetAgent(*agentID)
	if err != nil {
		return errors.New("agent not found")
	}
	if a.UserID != userID {
		return errors.New("agent not found")
	}
	return nil
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
		if err := validateViaAgent(d, u.ID, req.ViaAgentID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		c, err := d.Store.CreateConnection(d.Cipher, u.ID, store.ConnectionInput{
			Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
			DefaultDB: req.DefaultDB, TLS: req.TLS, Color: req.Color, GroupName: req.GroupName, Engine: req.Engine,
			SSHEnabled: req.SSHEnabled, SSHHost: req.SSHHost, SSHPort: req.SSHPort, SSHUser: req.SSHUser,
			SSHPassword: req.SSHPassword, SSHKey: req.SSHKey, SSHKeyPassphrase: req.SSHKeyPassphrase,
			ViaAgentID: req.ViaAgentID,
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
		if err := validateViaAgent(d, u.ID, req.ViaAgentID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		c, err := d.Store.UpdateConnection(d.Cipher, u.ID, id, store.ConnectionInput{
			Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
			DefaultDB: req.DefaultDB, TLS: req.TLS, Color: req.Color, GroupName: req.GroupName, Engine: req.Engine,
			SSHEnabled: req.SSHEnabled, SSHHost: req.SSHHost, SSHPort: req.SSHPort, SSHUser: req.SSHUser,
			SSHPassword: req.SSHPassword, SSHKey: req.SSHKey, SSHKeyPassphrase: req.SSHKeyPassphrase,
			ViaAgentID: req.ViaAgentID,
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
		d.Pool.Evict(db.PoolKey{UserID: u.ID, ConnID: id})
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
		d.Pool.Evict(db.PoolKey{UserID: u.ID, ConnID: id})
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
		dialect, err := dialectForConn(conn)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "unsupported engine: "+err.Error())
			return
		}
		pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "decrypt failed")
			return
		}
		ctx, cancel := contextWithTimeout(r.Context(), 5)
		defer cancel()
		if conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, &connSession{Conn: conn, Password: pw, Dialect: dialect}, conn.DefaultDB)
			if err != nil {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
				return
			}
			if _, err := exec.Run(ctx, "SELECT 1", mysqldialect.RunOpts{Database: conn.DefaultDB}); err != nil {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "connected"})
			return
		}
		dsnIn := db.DSNInput{
			Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
			DefaultDB: conn.DefaultDB, TLS: conn.TLS,
		}
		sshCfg := sshConfigFor(d, u.ID, conn)
		dbh, err := d.Pool.Get(db.PoolKey{UserID: u.ID, ConnID: id}, dialect, dsnIn, sshCfg)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		if err := dbh.PingContext(ctx); err != nil {
			d.Pool.Evict(db.PoolKey{UserID: u.ID, ConnID: id})
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "connected"})
	}
}

func handleTestConnectionDraft(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var req connectionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := req.validateTarget(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateViaAgent(d, u.ID, req.ViaAgentID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		pw := req.Password
		sshPassword := req.SSHPassword
		sshKey := req.SSHKey
		sshKeyPassphrase := req.SSHKeyPassphrase
		sshKeySet := sshKey != ""
		connID := int64(0)
		if req.ID != nil {
			stored, err := d.Store.GetConnection(u.ID, *req.ID)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					writeError(w, http.StatusNotFound, "not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "lookup failed")
				return
			}
			connID = stored.ID
			if pw == "" {
				if storedPW, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, stored.ID); err == nil {
					pw = storedPW
				}
			}
			if req.SSHEnabled {
				if sshKey == "" && stored.SSHKeySet {
					if key, pass, err := d.Store.GetSSHKey(d.Cipher, u.ID, stored.ID); err == nil {
						sshKey, sshKeyPassphrase = key, pass
						sshKeySet = key != ""
					}
				}
				if sshPassword == "" && !sshKeySet {
					if storedSSHPW, err := d.Store.GetSSHPassword(d.Cipher, u.ID, stored.ID); err == nil {
						sshPassword = storedSSHPW
					}
				}
			}
		}

		conn := store.Connection{
			ID: connID, UserID: u.ID, Name: req.Name, Host: req.Host, Port: req.Port,
			Username: req.Username, DefaultDB: req.DefaultDB, TLS: req.TLS,
			Color: req.Color, GroupName: req.GroupName, Engine: req.Engine,
			SSHEnabled: req.SSHEnabled, SSHHost: req.SSHHost, SSHPort: req.SSHPort,
			SSHUser: req.SSHUser, SSHKeySet: sshKeySet, ViaAgentID: req.ViaAgentID,
		}
		if conn.Port == 0 {
			conn.Port = 3306
		}
		if conn.TLS == "" {
			conn.TLS = "disabled"
		}
		if conn.Engine == "" {
			conn.Engine = "mysql"
		}
		if conn.SSHPort == 0 {
			conn.SSHPort = 22
		}

		ctx, cancel := contextWithTimeout(r.Context(), 5)
		defer cancel()
		sshCfg := draftSSHConfig(conn, sshPassword, sshKey, sshKeyPassphrase)
		ok, msg := testConnection(ctx, d, u.ID, conn, pw, sshCfg, db.PoolKey{UserID: u.ID, ConnID: -time.Now().UnixNano()})
		writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "message": msg})
	}
}

func draftSSHConfig(conn store.Connection, password, key, keyPassphrase string) db.SSHConfig {
	if !conn.SSHEnabled {
		return db.SSHConfig{}
	}
	cfg := db.SSHConfig{Host: conn.SSHHost, Port: conn.SSHPort, User: conn.SSHUser}
	if key != "" {
		cfg.PrivateKey = key
		cfg.KeyPassphrase = keyPassphrase
	} else {
		cfg.Password = password
	}
	return cfg
}

func testConnection(ctx context.Context, d Deps, userID int64, conn store.Connection, pw string, sshCfg db.SSHConfig, poolKey db.PoolKey) (bool, string) {
	dialect, err := dialectForConn(conn)
	if err != nil {
		return false, "unsupported engine: " + err.Error()
	}
	if conn.ViaAgentID != nil {
		if d.AgentRegistry == nil {
			return false, agent.ErrAgentOffline.Error()
		}
		ac, ok := d.AgentRegistry.Get(agent.AgentIDString(*conn.ViaAgentID))
		if !ok || ac.UserID != userID {
			return false, agent.ErrAgentOffline.Error()
		}
		exec := agent.AgentExecutor{
			Conn:    ac,
			Dialect: conn.Engine,
			Target: agent.MySQLTarget{
				Host: conn.Host, Port: conn.Port, User: conn.Username, Password: pw, Database: conn.DefaultDB,
				SSH: agentSSHConfig(sshCfg),
			},
		}
		if _, err := exec.Run(ctx, "SELECT 1", mysqldialect.RunOpts{Database: conn.DefaultDB}); err != nil {
			return false, err.Error()
		}
		return true, "connected"
	}
	dsnIn := db.DSNInput{
		Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
		DefaultDB: conn.DefaultDB, TLS: conn.TLS,
	}
	dbh, err := d.Pool.Get(poolKey, dialect, dsnIn, sshCfg)
	defer d.Pool.Evict(poolKey)
	if err != nil {
		return false, err.Error()
	}
	if err := dbh.PingContext(ctx); err != nil {
		return false, err.Error()
	}
	return true, "connected"
}
