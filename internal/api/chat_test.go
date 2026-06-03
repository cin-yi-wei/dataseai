package api

import (
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"
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
