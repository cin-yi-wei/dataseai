package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/conray/dataseai/internal/db"
	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
	"github.com/conray/dataseai/internal/store"
)

type wsReq struct {
	Type    string `json:"type"`
	QueryID string `json:"queryId"`
	ConnID  int64  `json:"connId"`
	DB      string `json:"db"`
	SQL     string `json:"sql"`
	MaxRows int    `json:"maxRows"`
}

type wsMsg struct {
	Type       string   `json:"type"`
	QueryID    string   `json:"queryId,omitempty"`
	Columns    []string `json:"cols,omitempty"`
	Batch      [][]any  `json:"batch,omitempty"`
	Offset     int      `json:"offset,omitempty"`
	Total      int64    `json:"total,omitempty"`
	DurationMs int64    `json:"durationMs,omitempty"`
	Message    string   `json:"message,omitempty"`
	Truncated  bool     `json:"truncated,omitempty"`
}

func clampQueryMaxRows(maxRows int) int {
	if maxRows <= 0 {
		return 10000
	}
	if maxRows > 10000 {
		return 10000
	}
	return maxRows
}

func handleWSQuery(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.URL.Query().Get("token")
		if tok == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		sess, err := d.Store.GetSession(tok)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		defer conn.CloseNow()

		ctx := r.Context()
		var currentCancel context.CancelFunc
		var currentQueryID string
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var req wsReq
			if err := json.Unmarshal(data, &req); err != nil {
				_ = wsjson.Write(context.Background(), conn, wsMsg{Type: "error", Message: "invalid json"})
				continue
			}
			switch req.Type {
			case "exec":
				if currentCancel != nil {
					currentCancel()
				}
				execCtx, cancel := context.WithCancel(ctx)
				currentCancel = cancel
				currentQueryID = req.QueryID
				_ = handleWSExec(execCtx, conn, d, sess.UserID, req)
				currentCancel = nil
				currentQueryID = ""
			case "cancel":
				if currentCancel != nil && currentQueryID == req.QueryID {
					currentCancel()
				}
				_ = wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: "canceled"})
			default:
				_ = wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: "unknown envelope type"})
			}
		}
	}
}

func handleWSExec(ctx context.Context, conn *websocket.Conn, d Deps, userID int64, req wsReq) error {
	if req.QueryID == "" {
		req.QueryID = "query"
	}
	if req.SQL == "" {
		return wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: "sql required"})
	}
	db, err := wsDBForUser(d, userID, req.ConnID)
	if err != nil {
		return wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: err.Error()})
	}
	sc, err := db.Conn(ctx)
	if err != nil {
		return wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: err.Error()})
	}
	defer sc.Close()
	if req.DB != "" {
		if _, err := sc.ExecContext(ctx, "USE "+d.Dialect.QuoteIdent(req.DB)); err != nil {
			return wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: err.Error()})
		}
	}
	var mysqlConnID int64
	_ = sc.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&mysqlConnID)
	d.QueryRegistry.Register(req.QueryID, mysqlConnID, req.SQL, userID, req.ConnID)
	defer d.QueryRegistry.Unregister(req.QueryID)

	start := time.Now()
	if mysqldialect.Classify(req.SQL) == mysqldialect.StmtExec {
		res, err := sc.ExecContext(ctx, req.SQL)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			_ = d.Store.AddHistoryWithCap(store.HistoryInput{
				UserID: userID, ConnectionID: req.ConnID, DatabaseName: req.DB,
				SQLText: req.SQL, DurationMs: dur, ErrorMessage: err.Error(), Source: "user",
			}, d.HistoryMax)
			return wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: err.Error()})
		}
		n, _ := res.RowsAffected()
		_ = d.Store.AddHistoryWithCap(store.HistoryInput{
			UserID: userID, ConnectionID: req.ConnID, DatabaseName: req.DB,
			SQLText: req.SQL, DurationMs: dur, RowsAffected: n, Source: "user",
		}, d.HistoryMax)
		return wsjson.Write(context.Background(), conn, wsMsg{Type: "done", QueryID: req.QueryID, Total: n, DurationMs: dur})
	}

	total, truncated, err := streamRowsOverWS(ctx, sc, conn, req.QueryID, req.SQL, clampQueryMaxRows(req.MaxRows))
	dur := time.Since(start).Milliseconds()
	if err != nil {
		_ = d.Store.AddHistoryWithCap(store.HistoryInput{
			UserID: userID, ConnectionID: req.ConnID, DatabaseName: req.DB,
			SQLText: req.SQL, DurationMs: dur, ErrorMessage: err.Error(), Source: "user",
		}, d.HistoryMax)
		return wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: err.Error()})
	}
	_ = d.Store.AddHistoryWithCap(store.HistoryInput{
		UserID: userID, ConnectionID: req.ConnID, DatabaseName: req.DB,
		SQLText: req.SQL, DurationMs: dur, RowsAffected: total, Source: "user",
	}, d.HistoryMax)
	return wsjson.Write(context.Background(), conn, wsMsg{Type: "done", QueryID: req.QueryID, Total: total, DurationMs: dur, Truncated: truncated})
}

func wsDBForUser(d Deps, userID, connID int64) (*sql.DB, error) {
	c, err := d.Store.GetConnection(userID, connID)
	if err != nil {
		return nil, err
	}
	pw, err := d.Store.GetConnectionPassword(d.Cipher, userID, connID)
	if err != nil {
		return nil, err
	}
	dsnIn := db.DSNInput{
		Host: c.Host, Port: c.Port, Username: c.Username, Password: pw,
		DefaultDB: c.DefaultDB, TLS: c.TLS,
	}
	sshCfg := sshConfigFor(d, userID, c)
	return d.Pool.Get(db.PoolKey{UserID: userID, ConnID: connID}, d.Dialect, dsnIn, sshCfg)
}

func streamRowsOverWS(ctx context.Context, sc *sql.Conn, conn *websocket.Conn, queryID, sqlText string, maxRows int) (int64, bool, error) {
	rows, err := sc.QueryContext(ctx, sqlText)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, false, err
	}
	if err := wsjson.Write(context.Background(), conn, wsMsg{Type: "columns", QueryID: queryID, Columns: cols}); err != nil {
		return 0, false, err
	}
	const batchSize = 100
	batch := make([][]any, 0, batchSize)
	offset := 0
	var total int64
	truncated := false
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return total, truncated, err
		}
		if total >= int64(maxRows) {
			truncated = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return total, truncated, err
		}
		for i, val := range vals {
			if b, ok := val.([]byte); ok {
				vals[i] = string(b)
			}
		}
		batch = append(batch, vals)
		total++
		if len(batch) >= batchSize {
			if err := wsjson.Write(context.Background(), conn, wsMsg{Type: "rows", QueryID: queryID, Batch: batch, Offset: offset}); err != nil {
				return total, truncated, err
			}
			offset += len(batch)
			batch = make([][]any, 0, batchSize)
		}
	}
	if err := rows.Err(); err != nil {
		return total, truncated, err
	}
	if len(batch) > 0 {
		if err := wsjson.Write(context.Background(), conn, wsMsg{Type: "rows", QueryID: queryID, Batch: batch, Offset: offset}); err != nil {
			return total, truncated, err
		}
	}
	return total, truncated, nil
}
