package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

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

func TestWS_ExecSelectStreamsRows(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	defer srv.Close()
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedDMLConn(t, r, tok)

	wsURL := "ws" + srv.URL[len("http"):] + "/ws/query?token=" + tok
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	req := map[string]any{
		"type": "exec", "queryId": "q1", "connId": connID, "sql": "SELECT 1 AS n",
	}
	data, _ := json.Marshal(req)
	if err := conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for i := 0; i < 3; i++ {
		_, msg, err := conn.Read(readCtx)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Type    string   `json:"type"`
			Columns []string `json:"cols"`
			Batch   [][]any  `json:"batch"`
			Total   int64    `json:"total"`
		}
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatal(err)
		}
		seen[got.Type] = true
		if got.Type == "columns" && (len(got.Columns) != 1 || got.Columns[0] != "n") {
			t.Fatalf("columns = %+v", got.Columns)
		}
		if got.Type == "rows" && (len(got.Batch) != 1 || len(got.Batch[0]) != 1) {
			t.Fatalf("batch = %+v", got.Batch)
		}
		if got.Type == "done" && got.Total != 1 {
			t.Fatalf("total = %d", got.Total)
		}
	}
	if !seen["columns"] || !seen["rows"] || !seen["done"] {
		t.Fatalf("missing ws events: %+v", seen)
	}
}

func TestWS_ExecSelectRespectsMaxRows(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	defer srv.Close()
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedDMLConn(t, r, tok)

	wsURL := "ws" + srv.URL[len("http"):] + "/ws/query?token=" + tok
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	req := map[string]any{
		"type": "exec", "queryId": "q1", "connId": connID,
		"sql":     "SELECT 1 AS n UNION ALL SELECT 2",
		"maxRows": 1,
	}
	data, _ := json.Marshal(req)
	if err := conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}

	readCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rowsSeen := 0
	doneSeen := false
	for !doneSeen {
		_, msg, err := conn.Read(readCtx)
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Type      string  `json:"type"`
			Batch     [][]any `json:"batch"`
			Total     int64   `json:"total"`
			Truncated bool    `json:"truncated"`
		}
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatal(err)
		}
		if got.Type == "rows" {
			rowsSeen += len(got.Batch)
		}
		if got.Type == "done" {
			doneSeen = true
			if got.Total != 1 {
				t.Fatalf("total = %d, want 1", got.Total)
			}
			if !got.Truncated {
				t.Fatal("truncated = false, want true")
			}
		}
	}
	if rowsSeen != 1 {
		t.Fatalf("rowsSeen = %d, want 1", rowsSeen)
	}
}
