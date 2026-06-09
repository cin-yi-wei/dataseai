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
	"github.com/conray/dataseai/internal/db"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

func parseConnID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "connId"), 10, 64)
}

// sshConfigFor builds the SSH tunnel config for a stored connection. Prefers
// private-key auth when a key is stored; falls back to password.
// Returns the zero SSHConfig when the connection has SSH disabled.
func sshConfigFor(d Deps, userID int64, conn store.Connection) db.SSHConfig {
	if !conn.SSHEnabled {
		return db.SSHConfig{}
	}
	cfg := db.SSHConfig{
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
	Conn     store.Connection
	Password string
	DB       *sql.DB
	Pool     *db.Pool
	Key      db.PoolKey
	// Dialect is the engine-specific dialect resolved from conn.Engine.
	// Always populated by the resolveConn* helpers (and the chat path) so
	// handlers don't have to look it up.
	Dialect db.Dialect
}

// dialectForConn looks up the dialect for a stored connection. Returns an
// error when the stored engine is unknown — that's a corrupt-DB / programmer
// bug, not a user error, and the caller should map it to a 500.
func dialectForConn(conn store.Connection) (db.Dialect, error) {
	engineStr := conn.Engine
	if engineStr == "" {
		engineStr = "mysql"
	}
	engine, err := db.ParseEngine(engineStr)
	if err != nil {
		return nil, err
	}
	dialect, ok := db.Lookup(engine)
	if !ok {
		return nil, errors.New("no dialect registered for engine " + string(engine))
	}
	return dialect, nil
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
	dialect, err := dialectForConn(conn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unsupported engine: "+err.Error())
		return nil, false
	}
	pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "decrypt failed")
		return nil, false
	}
	dsnIn := db.DSNInput{
		Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
		DefaultDB: conn.DefaultDB, TLS: conn.TLS,
	}
	sshCfg := sshConfigFor(d, u.ID, conn)
	key := db.PoolKey{UserID: u.ID, ConnID: id}
	dbh, err := d.Pool.Get(key, dialect, dsnIn, sshCfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pool open failed")
		return nil, false
	}
	return &connSession{Conn: conn, Password: pw, DB: dbh, Pool: d.Pool, Key: key, Dialect: dialect}, true
}

func resolveConnForRead(d Deps, w http.ResponseWriter, r *http.Request) (*connSession, bool) {
	id, err := parseConnID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad connId")
		return nil, false
	}
	return resolveConnByID(d, w, r, id)
}

func handleListDatabases(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConnForRead(d, w, r)
		if !ok {
			return
		}
		includeSystem := r.URL.Query().Get("system") == "1"
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, "")
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			names, err := listDatabasesViaExecutor(ctx, exec, includeSystem, cs.Dialect)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"databases": names})
			return
		}
		names, err := cs.Dialect.ListDatabases(ctx, cs.DB, includeSystem)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"databases": names})
	}
}

func handleListTables(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConnForRead(d, w, r)
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
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			tables, err := listTablesViaExecutor(ctx, exec, schema, cs.Dialect)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
			return
		}
		tables, err := cs.Dialect.ListTables(ctx, cs.DB, schema)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
	}
}

func handleDBSchema(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConnForRead(d, w, r)
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
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			cols, err := listSchemaColumnsViaExecutor(ctx, exec, schema, cs.Dialect)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"tables": cols})
			return
		}
		cols, err := cs.Dialect.ListSchemaColumns(ctx, cs.DB, schema)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tables": cols})
	}
}

func handleTableData(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConnForRead(d, w, r)
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

		var filters []db.Filter
		if f := q.Get("filters"); f != "" {
			if err := json.Unmarshal([]byte(f), &filters); err != nil {
				writeError(w, http.StatusBadRequest, "bad filters json")
				return
			}
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			out, err := fetchTableRowsViaExecutor(ctx, exec, db.RowsOpts{
				Schema: schema, Table: table, Page: page, PerPage: perPage,
				SortCol: q.Get("sort_col"), SortDir: q.Get("sort_dir"),
				Filters: filters,
			}, cs.Dialect)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		out, err := cs.Dialect.FetchTableRows(ctx, cs.DB, db.RowsOpts{
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
		cs, ok := resolveConnForRead(d, w, r)
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
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			out, err := describeTableViaExecutor(ctx, exec, schema, table, cs.Dialect)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "describe failed")
				return
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
		out, err := cs.Dialect.DescribeTable(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "describe failed")
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleIndexes(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConnForRead(d, w, r)
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
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			out, err := listIndexesViaExecutor(ctx, exec, schema, table, cs.Dialect)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "indexes failed")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"indexes": out})
			return
		}
		out, err := cs.Dialect.ListIndexes(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "indexes failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"indexes": out})
	}
}

func handleFKs(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cs, ok := resolveConnForRead(d, w, r)
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
		if cs.Conn.ViaAgentID != nil {
			exec, err := executorForQuery(d, cs, schema)
			if err != nil {
				writeError(w, queryStatusForError(err), err.Error())
				return
			}
			out, err := listForeignKeysViaExecutor(ctx, exec, schema, table, cs.Dialect)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "fks failed")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"fks": out})
			return
		}
		out, err := cs.Dialect.ListForeignKeys(ctx, cs.DB, schema, table)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "fks failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"fks": out})
	}
}
