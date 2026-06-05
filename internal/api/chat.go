package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/conray/dataseai/internal/chat"
	"github.com/conray/dataseai/internal/llm"
	"github.com/conray/dataseai/internal/mysql"
	"github.com/conray/dataseai/internal/store"
)

type chatExecReq struct {
	Type     string        `json:"type"`              // "exec" | "execute_write" | "cancel"
	ConnID   int64         `json:"conn_id,omitempty"`
	DB       string        `json:"db,omitempty"`
	Provider string        `json:"provider,omitempty"` // "anthropic" | "openai" | ""
	Messages []llm.Message `json:"messages,omitempty"`

	// execute_write envelopes
	ProposalID string `json:"proposal_id,omitempty"`
	Accept     bool   `json:"accept,omitempty"`
}

type chatMsg struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolInput any    `json:"tool_input,omitempty"`
	Output    string `json:"output,omitempty"`
	Message   string `json:"message,omitempty"`

	// write_proposed / write_executed / write_failed / write_cancelled
	ProposalID     string `json:"proposal_id,omitempty"`
	Database       string `json:"database,omitempty"`
	Table          string `json:"table,omitempty"`
	Operation      string `json:"operation,omitempty"`
	SQL            string `json:"sql,omitempty"`
	ExplainSummary string `json:"explain_summary,omitempty"`
	RowsAffected   int64  `json:"rows_affected,omitempty"`
}

// pendingState tracks in-flight proposals waiting for a user decision.
type pendingState struct {
	mu    sync.Mutex
	chans map[string]chan chat.Decision
}

// wsGateway implements chat.ProposalGateway over a WebSocket connection.
type wsGateway struct {
	write   func(chatMsg) error
	pending *pendingState
}

func (g wsGateway) Propose(ctx context.Context, p chat.Proposal) (chat.Decision, error) {
	ch := make(chan chat.Decision, 1)
	g.pending.mu.Lock()
	g.pending.chans[p.ID] = ch
	g.pending.mu.Unlock()
	defer func() {
		g.pending.mu.Lock()
		delete(g.pending.chans, p.ID)
		g.pending.mu.Unlock()
	}()
	if err := g.write(chatMsg{
		Type:           "write_proposed",
		ProposalID:     p.ID,
		Database:       p.Database,
		Table:          p.Table,
		Operation:      p.Operation,
		SQL:            p.SQL,
		ExplainSummary: p.ExplainSummary,
	}); err != nil {
		return chat.Decision{}, err
	}
	select {
	case d := <-ch:
		return d, nil
	case <-ctx.Done():
		return chat.Decision{}, ctx.Err()
	}
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

		// Build the per-session gateway before starting the orchestrator.
		pending := &pendingState{chans: map[string]chan chat.Decision{}}
		gw := wsGateway{
			write:   func(m chatMsg) error { return wsjson.Write(ctx, c, m) },
			pending: pending,
		}

		// Goroutine: read follow-up envelopes (execute_write / cancel).
		// The orchestrator's event loop is the only *writer* of results; this
		// goroutine is the only *reader* of follow-up messages, so there is no
		// concurrent read conflict on the WebSocket connection.
		go func() {
			for {
				var m chatExecReq
				if err := wsjson.Read(ctx, c, &m); err != nil {
					return
				}
				switch m.Type {
				case "execute_write":
					pending.mu.Lock()
					ch, ok := pending.chans[m.ProposalID]
					pending.mu.Unlock()
					if ok {
						ch <- chat.Decision{Accept: m.Accept}
					}
				case "cancel":
					cancel()
					return
				}
			}
		}()

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
		dsnIn := mysql.DSNInput{
			Host: conn.Host, Port: conn.Port, Username: conn.Username, Password: pw,
			DefaultDB: conn.DefaultDB, TLS: conn.TLS,
		}
		sshCfg := sshConfigFor(d, u.ID, conn)
		db, err := d.Pool.Get(mysql.PoolKey{UserID: u.ID, ConnID: req.ConnID}, dsnIn, sshCfg)
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

		// Per-user keys take precedence over server defaults.
		effectiveCfg := d.LLMConfig
		if userKeys, err := d.Store.GetUserAPIKeys(d.Cipher, u.ID); err == nil {
			if userKeys.Anthropic != "" {
				effectiveCfg.AnthropicAPIKey = userKeys.Anthropic
			}
			if userKeys.OpenAI != "" {
				effectiveCfg.OpenAIAPIKey = userKeys.OpenAI
			}
			if userKeys.Gemini != "" {
				effectiveCfg.GeminiAPIKey = userKeys.Gemini
			}
		}
		// Codex (ChatGPT subscription): per-user OAuth tokens, refresh on
		// expiry, derive account_id from the access_token JWT.
		if cx, err := d.Store.GetCodexTokens(d.Cipher, u.ID); err == nil && cx.AccessToken != "" {
			access := cx.AccessToken
			if cx.RefreshToken != "" && cx.ExpiresAtMs > 0 && time.Now().UnixMilli() > cx.ExpiresAtMs-5*60*1000 {
				if t, rerr := llm.RefreshCodexTokens(nil, cx.RefreshToken); rerr == nil {
					newExp := int64(0)
					if t.ExpiresIn > 0 {
						newExp = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).UnixMilli()
					}
					rt := t.RefreshToken
					if rt == "" {
						rt = cx.RefreshToken
					}
					_ = d.Store.SetCodexTokens(d.Cipher, u.ID, store.CodexTokens{
						AccessToken: t.AccessToken, RefreshToken: rt, ExpiresAtMs: newExp,
					})
					access = t.AccessToken
				}
			}
			effectiveCfg.CodexToken = access
			if acct, err := llm.ExtractCodexAccountID(access); err == nil {
				effectiveCfg.CodexAccountID = acct
			}
		}

		// Claude Code: read the full OAuth bundle, refresh on expiry, persist
		// rotated tokens back. The legacy single-token column (set via the
		// paste-only Settings field) is what UserAPIKeys.ClaudeCode would
		// surface; this newer path supersedes it whenever a refresh token is
		// stored too.
		if cc, err := d.Store.GetClaudeCodeTokens(d.Cipher, u.ID); err == nil && cc.AccessToken != "" {
			access := cc.AccessToken
			if cc.RefreshToken != "" && cc.ExpiresAtMs > 0 {
				expiringSoon := time.Now().UnixMilli() > cc.ExpiresAtMs-5*60*1000
				if expiringSoon {
					if t, rerr := llm.RefreshTokens(nil, cc.RefreshToken); rerr == nil {
						newExp := int64(0)
						if t.ExpiresIn > 0 {
							newExp = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).UnixMilli()
						}
						rt := t.RefreshToken
						if rt == "" {
							rt = cc.RefreshToken // some refresh responses omit rotation
						}
						_ = d.Store.SetClaudeCodeTokens(d.Cipher, u.ID, store.ClaudeCodeTokens{
							AccessToken: t.AccessToken, RefreshToken: rt, ExpiresAtMs: newExp,
						})
						access = t.AccessToken
					}
				}
			}
			effectiveCfg.ClaudeCodeToken = access
		}
		llmClient, err := llm.Pick(effectiveCfg, req.Provider)
		if err != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: err.Error()})
			return
		}

		// Read the master AI-writes switch once at session start.
		masterOn, _ := d.Store.GetAIWritesEnabled(u.ID)

		events, runErr := chat.Run(ctx, chat.Deps{
			LLM: llmClient, DB: db, Store: d.Store, Gateway: gw,
			UserID: u.ID, ConnID: req.ConnID, DefaultDB: req.DB,
			IncludeProposeWrite: masterOn,
		}, chat.Input{Messages: req.Messages})
		if runErr != nil {
			_ = wsjson.Write(ctx, c, chatMsg{Type: "error", Message: runErr.Error()})
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
