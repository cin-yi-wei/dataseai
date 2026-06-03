package api

import (
	"context"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/conray/mysqlweb/internal/chat"
	"github.com/conray/mysqlweb/internal/llm"
	"github.com/conray/mysqlweb/internal/mysql"
	"github.com/conray/mysqlweb/internal/store"
)

type chatExecReq struct {
	Type     string        `json:"type"`     // "exec" | "cancel"
	ConnID   int64         `json:"conn_id"`
	DB       string        `json:"db"`
	Provider string        `json:"provider"` // "anthropic" | "openai" | ""
	Messages []llm.Message `json:"messages"`
}

type chatMsg struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolInput any    `json:"tool_input,omitempty"`
	Output    string `json:"output,omitempty"`
	Message   string `json:"message,omitempty"`
}

func handleWSChat(d Deps) http.HandlerFunc {
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
		u, err := d.Store.GetUserByID(sess.UserID)
		if err != nil {
			http.Error(w, "invalid user", http.StatusUnauthorized)
			return
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
		if err != nil {
			return
		}
		defer c.CloseNow()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		var req chatExecReq
		if err := wsjson.Read(ctx, c, &req); err != nil {
			return
		}
		if req.Type != "exec" {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: "first envelope must be type:exec"})
			return
		}

		// Resolve the user's connection + open the *sql.DB.
		conn, err := d.Store.GetConnection(u.ID, req.ConnID)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: "connection not found"})
			return
		}
		pw, err := d.Store.GetConnectionPassword(d.Cipher, u.ID, req.ConnID)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: "decrypt failed"})
			return
		}
		dsn := mysql.BuildDSN(mysql.DSNInput{
			Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
			DefaultDB: conn.DefaultDB, TLS: conn.TLS,
		})
		db, err := d.Pool.Get(mysql.PoolKey{UserID: u.ID, ConnID: req.ConnID}, dsn)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: err.Error()})
			return
		}
		if req.DB != "" {
			sc, err := db.Conn(ctx)
			if err == nil {
				_, _ = sc.ExecContext(ctx, "USE "+mysql.QuoteIdent(req.DB))
				_ = sc.Close()
			}
		}

		llmClient, err := llm.Pick(d.LLMConfig, req.Provider)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: err.Error()})
			return
		}
		events, err := chat.Run(ctx, chat.Deps{LLM: llmClient, DB: db}, chat.Input{
			Messages: req.Messages,
		})
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: err.Error()})
			return
		}
		for ev := range events {
			out := chatMsg{Type: ev.Type, Text: ev.Text, ToolUseID: ev.ToolUseID,
				ToolName: ev.ToolName, ToolInput: ev.ToolInput, Output: ev.Output, Message: ev.Message}
			if err := wsjson.Write(ctx, c, out); err != nil {
				return
			}
			if ev.Type == llm.EventDone || ev.Type == llm.EventError {
				return
			}
		}
		_ = store.Connection{} // keep store import live
	}
}
