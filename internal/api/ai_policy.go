package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
)

// scopeFromRequest pulls ?scope=ai|dml from the URL (or empty for default ai).
func scopeFromRequest(r *http.Request) store.PolicyScope {
	return store.NormalizeScope(r.URL.Query().Get("scope"))
}

type writesEnabledResp struct {
	Enabled bool   `json:"enabled"`
	Scope   string `json:"scope"`
}

// handleGetAIWrites and handlePutAIWrites are now scope-aware: ?scope=dml
// routes to the DataGrid master switch; everything else still gates AI.
func handleGetAIWrites(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		scope := scopeFromRequest(r)
		en, err := d.Store.GetWritesEnabled(u.ID, scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, writesEnabledResp{Enabled: en, Scope: string(scope)})
	}
}

func handlePutAIWrites(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		scope := scopeFromRequest(r)
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		var err error
		if scope == store.ScopeDML {
			err = d.Store.SetDMLWritesEnabled(u.ID, body.Enabled)
		} else {
			err = d.Store.SetAIWritesEnabled(u.ID, body.Enabled)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, writesEnabledResp{Enabled: body.Enabled, Scope: string(scope)})
	}
}

type aiPolicyBody struct {
	Conn   int64          `json:"conn"`
	DB     string         `json:"db"`
	Table  string         `json:"table"`
	Policy store.AIPolicy `json:"policy"`
}

func handlePutAIPolicy(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		scope := scopeFromRequest(r)
		var body aiPolicyBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Conn == 0 || body.DB == "" || body.Table == "" {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		if _, err := d.Store.GetConnection(u.ID, body.Conn); err != nil {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		if err := d.Store.UpsertWritePolicy(u.ID, body.Conn, body.DB, body.Table, scope, body.Policy); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"table": body.Table, "policy": body.Policy, "scope": string(scope)})
	}
}

type aiPolicyBatchBody struct {
	Conn   int64          `json:"conn"`
	DB     string         `json:"db"`
	Tables []string       `json:"tables"`
	Policy store.AIPolicy `json:"policy"`
}

func handleBatchAIPolicy(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		scope := scopeFromRequest(r)
		var body aiPolicyBatchBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Conn == 0 || body.DB == "" || len(body.Tables) == 0 {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		if _, err := d.Store.GetConnection(u.ID, body.Conn); err != nil {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		if err := d.Store.BatchUpsertWritePolicy(u.ID, body.Conn, body.DB, body.Tables, scope, body.Policy); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"updated": len(body.Tables), "scope": string(scope)})
	}
}

func handleDeleteAIPolicy(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		scope := scopeFromRequest(r)
		q := r.URL.Query()
		connStr, db, table := q.Get("conn"), q.Get("db"), q.Get("table")
		connID, err := strconv.ParseInt(connStr, 10, 64)
		if err != nil || db == "" || table == "" {
			writeError(w, http.StatusBadRequest, "bad query")
			return
		}
		if err := d.Store.DeleteWritePolicy(u.ID, connID, db, table, scope); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleListAIPolicy(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		scope := scopeFromRequest(r)
		connID, err := strconv.ParseInt(r.URL.Query().Get("conn"), 10, 64)
		db := r.URL.Query().Get("db")
		if err != nil || db == "" {
			writeError(w, http.StatusBadRequest, "bad query")
			return
		}
		if _, err := d.Store.GetConnection(u.ID, connID); err != nil {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		configured, err := d.Store.ListWritePolicy(u.ID, connID, db, scope)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		all, err := listAllTablesForAIPolicy(r.Context(), d, u.ID, connID, db)
		if err != nil {
			writeError(w, http.StatusBadGateway, "list tables: "+err.Error())
			return
		}
		configuredSet := map[string]struct{}{}
		for _, c := range configured {
			configuredSet[c.Table] = struct{}{}
		}
		var unconfigured []string
		for _, name := range all {
			if _, ok := configuredSet[name]; !ok {
				unconfigured = append(unconfigured, name)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"configured":   configured,
			"unconfigured": unconfigured,
		})
	}
}

func handleListAIAudit(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}
		scope := scopeFromRequest(r)
		rows, err := d.Store.RecentAuditByScope(u.ID, scope, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

// listAllTablesForAIPolicy lists every user-visible table on a connection's
// database so the policy UI can show "unconfigured" tables alongside the
// ones that already have a policy row.
func listAllTablesForAIPolicy(ctx context.Context, d Deps, userID, connID int64, db string) ([]string, error) {
	if d.Pool == nil {
		return nil, nil
	}
	conn, err := d.Store.GetConnection(userID, connID)
	if err != nil {
		return nil, err
	}
	pw, err := d.Store.GetConnectionPassword(d.Cipher, userID, connID)
	if err != nil {
		return nil, err
	}
	dsn := mysql.DSNInput{
		Host:      conn.Host,
		Port:      conn.Port,
		Username:  conn.Username,
		Password:  pw,
		DefaultDB: conn.DefaultDB,
		TLS:       conn.TLS,
	}
	var ssh mysql.SSHConfig
	if conn.SSHEnabled {
		sshPw, _ := d.Store.GetSSHPassword(d.Cipher, userID, connID)
		ssh = mysql.SSHConfig{Host: conn.SSHHost, Port: conn.SSHPort, User: conn.SSHUser, Password: sshPw}
	}
	pool, err := d.Pool.Get(mysql.PoolKey{UserID: userID, ConnID: connID}, dsn, ssh)
	if err != nil {
		return nil, err
	}
	tableInfos, err := mysql.ListTables(ctx, pool, db)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tableInfos))
	for _, ti := range tableInfos {
		names = append(names, ti.Name)
	}
	return names, nil
}
