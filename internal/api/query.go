package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/conray/dataseai/internal/agent"
	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
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
	if conn.ViaAgentID != nil {
		return &connSession{Conn: conn, Password: pw}, true
	}
	dsnIn := mysql.DSNInput{
		Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
		DefaultDB: conn.DefaultDB, TLS: conn.TLS,
	}
	sshCfg := sshConfigFor(d, u.ID, conn)
	key := mysql.PoolKey{UserID: u.ID, ConnID: connID}
	db, err := d.Pool.Get(key, dsnIn, sshCfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pool open failed")
		return nil, false
	}
	return &connSession{Conn: conn, Password: pw, DB: db, Pool: d.Pool, Key: key}, true
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
		exec, err := executorForQuery(d, cs, req.DatabaseName)
		if err != nil {
			writeError(w, queryStatusForError(err), err.Error())
			return
		}
		out, err := exec.Run(ctx, req.SQL, mysql.RunOpts{
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

func executorForQuery(d Deps, cs *connSession, databaseName string) (mysql.Executor, error) {
	if cs.Conn.ViaAgentID == nil {
		return mysql.DirectExecutor{DB: cs.DB}, nil
	}
	if d.AgentRegistry == nil {
		return nil, agent.ErrAgentOffline
	}
	ac, ok := d.AgentRegistry.Get(agent.AgentIDString(*cs.Conn.ViaAgentID))
	if !ok {
		return nil, agent.ErrAgentOffline
	}
	if ac.UserID != cs.Conn.UserID {
		return nil, agent.ErrAgentOffline
	}
	dbName := cs.Conn.DefaultDB
	if databaseName != "" {
		dbName = databaseName
	}
	return agent.AgentExecutor{
		Conn: ac,
		Target: agent.MySQLTarget{
			Host: cs.Conn.Host, Port: cs.Conn.Port, User: cs.Conn.Username, Password: cs.Password, Database: dbName,
		},
	}, nil
}

func queryStatusForError(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusRequestTimeout
	}
	if errors.Is(err, agent.ErrAgentOffline) {
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}
