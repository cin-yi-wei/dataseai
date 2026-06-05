package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

func parseConnID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "connId"), 10, 64)
}

// sshConfigFor builds the SSH tunnel config for a stored connection. Prefers
// private-key auth when a key is stored; falls back to password.
// Returns the zero SSHConfig when the connection has SSH disabled.
func sshConfigFor(d Deps, userID int64, conn store.Connection) mysql.SSHConfig {
	if !conn.SSHEnabled {
		return mysql.SSHConfig{}
	}
	cfg := mysql.SSHConfig{
		Host: conn.SSHHost, Port: conn.SSHPort, User: conn.SSHUser,
	}
	if conn.SSHKeySet {
		key, pass, _ := d.Store.GetSSHKey(d.Cipher, userID, conn.ID)
		cfg.PrivateKey = key
		cfg.KeyPassphrase = pass
	} else {
		pw, _ := d.Store.GetSSHPassword(d.Cipher, userID, conn.ID)
		cfg.Password = pw
	}
	return cfg
}

type connSession struct {
	Conn store.Connection
	DB   *sql.DB
	Pool *mysql.Pool
	Key  mysql.PoolKey
}

func resolveConn(d Deps, w http.ResponseWriter, r *http.Request) (*connSession, bool) {
	u, _ := auth.UserFromContext(r.Context())
	id, err := parseConnID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad connId")
		return nil, false
	}
	conn, err := d.Store.GetConnection(u.ID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connection not found")
		} else {
			writeError(w, http.StatusInternalServerError, "lookup failed")
		}
		return nil, false
	}
	pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "decrypt failed")
		return nil, false
	}
	dsnIn := mysql.DSNInput{
		Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
		DefaultDB: conn.DefaultDB, TLS: conn.TLS,
	}
	sshCfg := sshConfigFor(d, u.ID, conn)
	key := mysql.PoolKey{UserID: u.ID, ConnID: id}
	db, err := d.Pool.Get(key, dsnIn, sshCfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pool open failed")
		return nil, false
	}
	return &connSession{Conn: conn, DB: db, Pool: d.Pool, Key: key}, true
}

func handleListDatabases(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		includeSystem := r.URL.Query().Get("system") == "1"
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		names, err := mysql.ListDatabases(ctx, cs.DB, includeSystem)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"databases": names})
	}
}

func handleListTables(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		if schema == "" {
			writeError(w, http.StatusBadRequest, "missing db")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		tables, err := mysql.ListTables(ctx, cs.DB, schema)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
	}
}

func handleDBSchema(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		if schema == "" {
			writeError(w, http.StatusBadRequest, "missing db")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		cols, err := mysql.ListSchemaColumns(ctx, cs.DB, schema)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tables": cols})
	}
}

func handleTableData(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if schema == "" || table == "" {
			writeError(w, http.StatusBadRequest, "missing db/table")
			return
		}
		q := r.URL.Query()
		page, _ := strconv.Atoi(q.Get("page"))
		perPage, _ := strconv.Atoi(q.Get("per_page"))

		var filters []mysql.Filter
		if f := q.Get("filters"); f != "" {
			if err := json.Unmarshal([]byte(f), &filters); err != nil {
				writeError(w, http.StatusBadRequest, "bad filters json")
				return
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.FetchTableRows(ctx, cs.DB, mysql.RowsOpts{
			Schema: schema, Table: table, Page: page, PerPage: perPage,
			SortCol: q.Get("sort_col"), SortDir: q.Get("sort_dir"),
			Filters: filters,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleStructure(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if schema == "" || table == "" {
			writeError(w, http.StatusBadRequest, "missing db/table")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.DescribeTable(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "describe failed")
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleIndexes(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if schema == "" || table == "" {
			writeError(w, http.StatusBadRequest, "missing db/table")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.ListIndexes(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "indexes failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"indexes": out})
	}
}

func handleFKs(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConn(d, w, r)
		if !ok {
			return
		}
		schema := chi.URLParam(r, "db")
		table := chi.URLParam(r, "table")
		if schema == "" || table == "" {
			writeError(w, http.StatusBadRequest, "missing db/table")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.ListForeignKeys(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "fks failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"fks": out})
	}
}
