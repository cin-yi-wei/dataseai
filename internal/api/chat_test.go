package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestWSChat_RequiresToken(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	defer srv.Close()
	wsURL := "ws" + srv.URL[len("http"):] + "/ws/chat"
	_, _, err := websocket.Dial(t.Context(), wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail without token")
	}
}

func TestWSChat_ViaAgentRequiresOnlineAgent(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tok := registerAndLoginURL(t, srv.URL, "alice", "supersecret123")
	agentRec := postURL(t, srv.URL, "/api/auth/agents", map[string]any{"name": "windows"}, tok)
	var createdAgent struct {
		Agent struct{ ID int64 } `json:"agent"`
	}
	_ = json.NewDecoder(agentRec.Body).Decode(&createdAgent)

	connRec := postURL(t, srv.URL, "/api/connections", map[string]any{
		"name": "via-agent", "host": "127.0.0.1", "port": 3306,
		"username": "root", "password": "pw", "via_agent_id": createdAgent.Agent.ID,
	}, tok)
	var createdConn struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(connRec.Body).Decode(&createdConn)

	wsURL := "ws" + srv.URL[len("http"):] + "/ws/chat?token=" + tok
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	if err := wsjson.Write(ctx, c, chatExecReq{
		Type:   "exec",
		ConnID: createdConn.Connection.ID,
		DB:     "test_windows",
	}); err != nil {
		t.Fatal(err)
	}
	var msg chatMsg
	if err := wsjson.Read(ctx, c, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "error" || msg.Message != "agent offline" {
		t.Fatalf("msg = %+v, want agent offline error", msg)
	}
}
