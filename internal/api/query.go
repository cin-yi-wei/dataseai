package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/conray/mysqlweb/internal/auth"
	"github.com/conray/mysqlweb/internal/mysql"
	"github.com/conray/mysqlweb/internal/store"
)

type queryReq struct {
	ConnID       int64  `json:"conn_id"`
	DatabaseName string `json:"database_name"`
	SQL          string `json:"sql"`
}

func resolveConnByID(d Deps, w http.ResponseWriter, r *http.Request, connID int64) (*connSession, bool) {
	u, _ := auth.UserFromContext(r.Context())
	conn, err := d.Store.GetConnection(u.ID, connID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "connection not found")
		} else {
			writeError(w, http.StatusInternalServerError, "lookup failed")
		}
		return nil, false
	}
	pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, connID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "decrypt failed")
		return nil, false
	}
	dsn := mysql.BuildDSN(mysql.DSNInput{
		Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
		DefaultDB: conn.DefaultDB, TLS: conn.TLS,
	})
	key := mysql.PoolKey{UserID: u.ID, ConnID: connID}
	db, err := d.Pool.Get(key, dsn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pool open failed")
		return nil, false
	}
	return &connSession{Conn: conn, DB: db, Pool: d.Pool, Key: key}, true
}

func handleQuery(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var req queryReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.SQL == "" {
			writeError(w, http.StatusBadRequest, "sql required")
			return
		}
		cs, ok := resolveConnByID(d, w, r, req.ConnID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(d.QueryTimeoutS)*time.Second)
		defer cancel()
		start := time.Now()
		out, err := mysql.Run(ctx, cs.DB, req.SQL, mysql.RunOpts{
			Database: req.DatabaseName,
		})
		dur := time.Since(start).Milliseconds()

		// Always record in history, success or failure
		entry := store.HistoryInput{
			UserID: u.ID, ConnectionID: req.ConnID,
			DatabaseName: req.DatabaseName, SQLText: req.SQL,
			DurationMs: dur, Source: "user",
		}
		if err != nil {
			entry.ErrorMessage = err.Error()
		} else {
			entry.RowsAffected = out.RowsAffected
		}
		_ = d.Store.AddHistoryWithCap(entry, d.HistoryMax)

		if err != nil {
			writeError(w, queryStatusForError(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"columns":       out.Columns,
			"rows":          out.Rows,
			"rows_affected": out.RowsAffected,
			"duration_ms":   dur,
			"truncated":     out.Truncated,
		})
	}
}

func queryStatusForError(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusRequestTimeout
	}
	return http.StatusInternalServerError
}
