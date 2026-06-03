package api

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
)

func TestWS_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + srv.URL[len("http"):] + "/ws/query"
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err == nil {
		conn.CloseNow()
		t.Fatal("expected dial to fail without token")
	}
}

func TestWS_AcceptsValidToken(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	defer srv.Close()
	tok := registerAndLogin(t, r, "alice", "supersecret123")

	wsURL := "ws" + srv.URL[len("http"):] + "/ws/query?token=" + tok
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	if err := conn.Write(context.Background(), websocket.MessageText, []byte(`not-json`)); err != nil {
		t.Fatal(err)
	}
	_, msg, err := conn.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) == "" {
		t.Fatal("empty reply")
	}
}
