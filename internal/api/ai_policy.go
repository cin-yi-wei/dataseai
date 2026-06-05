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

type aiWritesResp struct {
	Enabled bool `json:"enabled"`
}

func handleGetAIWrites(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		en, err := d.Store.GetAIWritesEnabled(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, aiWritesResp{Enabled: en})
	}
}

func handlePutAIWrites(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		if err := d.Store.SetAIWritesEnabled(u.ID, body.Enabled); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, aiWritesResp{Enabled: body.Enabled})
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
		var body aiPolicyBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Conn == 0 || body.DB == "" || body.Table == "" {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		if _, err := d.Store.GetConnection(u.ID, body.Conn); err != nil {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		if err := d.Store.UpsertAIPolicy(u.ID, body.Conn, body.DB, body.Table, body.Policy); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"table": body.Table, "policy": body.Policy})
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
		var body aiPolicyBatchBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Conn == 0 || body.DB == "" || len(body.Tables) == 0 {
			writeError(w, http.StatusBadRequest, "bad body")
			return
		}
		if _, err := d.Store.GetConnection(u.ID, body.Conn); err != nil {
			writeError(w, http.StatusNotFound, "connection not found")
			return
		}
		if err := d.Store.BatchUpsertAIPolicy(u.ID, body.Conn, body.DB, body.Tables, body.Policy); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"updated": len(body.Tables)})
	}
}

func handleDeleteAIPolicy(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		q := r.URL.Query()
		connStr, db, table := q.Get("conn"), q.Get("db"), q.Get("table")
		connID, err := strconv.ParseInt(connStr, 10, 64)
		if err != nil || db == "" || table == "" {
			writeError(w, http.StatusBadRequest, "bad query")
			return
		}
		if err := d.Store.DeleteAIPolicy(u.ID, connID, db, table); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleListAIPolicy(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
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
		configured, err := d.Store.ListAIPolicy(u.ID, connID, db)
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
		rows, err := d.Store.RecentAIAudit(u.ID, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

// listAllTablesForAIPolicy opens the user's MySQL pool entry and returns
// every table name in `db`. Used by handleListAIPolicy to compute the
// "unconfigured" list. Returns (nil, nil) if pool isn't available in tests.
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
