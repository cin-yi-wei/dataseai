package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type wsReq struct {
	Type    string `json:"type"`
	QueryID string `json:"queryId"`
	ConnID  int64  `json:"connId"`
	DB      string `json:"db"`
	SQL     string `json:"sql"`
}

type wsMsg struct {
	Type    string `json:"type"`
	QueryID string `json:"queryId,omitempty"`
	Message string `json:"message,omitempty"`
}

func handleWSQuery(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.URL.Query().Get("token")
		if tok == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		if _, err := d.Store.GetSession(tok); err != nil {
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
			_ = wsjson.Write(context.Background(), conn, wsMsg{Type: "error", QueryID: req.QueryID, Message: "unknown envelope type"})
		}
	}
}
