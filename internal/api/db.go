package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/mysql"
	"github.com/conray/mysqlweb/internal/store"
	"github.com/go-chi/chi/v5"
)

func parseConnID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "connId"), 10, 64)
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
	dsn := mysql.BuildDSN(mysql.DSNInput{
		Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
		DefaultDB: conn.DefaultDB, TLS: conn.TLS,
	})
	key := mysql.PoolKey{UserID: u.ID, ConnID: id}
	db, err := d.Pool.Get(key, dsn)
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		names, err := mysql.ListDatabases(ctx, cs.DB)
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
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		out, err := mysql.FetchTableRows(ctx, cs.DB, mysql.RowsOpts{
			Schema: schema, Table: table, Page: page, PerPage: perPage,
			SortCol: q.Get("sort_col"), SortDir: q.Get("sort_dir"),
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
