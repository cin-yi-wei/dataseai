package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/conray/dataseai/internal/agent"
	"github.com/conray/dataseai/internal/store"
)

func postURL(t *testing.T, baseURL, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(buf))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	rec := httptest.NewRecorder()
	rec.Code = resp.StatusCode
	_, _ = rec.Body.ReadFrom(resp.Body)
	return rec
}

func registerAndLoginURL(t *testing.T, baseURL, username, password string) string {
	t.Helper()
	rec := postURL(t, baseURL, "/api/auth/register", map[string]string{"username": username, "password": password}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
	}
	var body struct{ Token string }
	_ = json.NewDecoder(rec.Body).Decode(&body)
	return body.Token
}

func remarshalPayload(payload any, dest any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func TestQuery_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := post(t, r, "/api/query", map[string]any{"conn_id": 1, "sql": "SELECT 1"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestQuery_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/query", map[string]any{"conn_id": 999, "sql": "SELECT 1"}, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestQuery_EmptySQL(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = post(t, r, "/api/query", map[string]any{"conn_id": created.Connection.ID, "sql": ""}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestQuery_HistoryIsWritten(t *testing.T) {
	r, s := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)

	// SELECT 1 works against sqlite — Run will succeed.
	rec = post(t, r, "/api/query", map[string]any{
		"conn_id":       created.Connection.ID,
		"database_name": "",
		"sql":           "SELECT 1",
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	// History row should exist
	var n int
	if err := s.DB.QueryRow("SELECT count(*) FROM query_history WHERE user_id=?", userIDOfAlice(s)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("history rows = %d, want 1", n)
	}
}

func TestQuery_HistoryRecordsFailures(t *testing.T) {
	r, s := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/connections", map[string]any{"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	// SELECT from non-existent table — sqlite returns "no such table"
	rec = post(t, r, "/api/query", map[string]any{
		"conn_id": created.Connection.ID, "sql": "SELECT * FROM no_such_table",
	}, tok)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d", rec.Code)
	}
	var errMsg string
	if err := s.DB.QueryRow("SELECT error_message FROM query_history WHERE user_id=? ORDER BY id DESC LIMIT 1", userIDOfAlice(s)).Scan(&errMsg); err != nil {
		t.Fatal(err)
	}
	if errMsg == "" {
		t.Fatal("error_message empty for failed query")
	}
}

func TestQuery_ViaAgentOfflineReturnsBadGateway(t *testing.T) {
	reg := agent.NewRegistry()
	r, s := newTestRouterWithSqliteAsMySQLDeps(t, func(d Deps) Deps {
		d.AgentRegistry = reg
		return d
	})
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	uID := userIDOfAlice(s)
	a, _, err := s.CreateAgent(uID, "windows")
	if err != nil {
		t.Fatal(err)
	}
	rec := post(t, r, "/api/connections", map[string]any{
		"name": "via-agent", "host": "127.0.0.1", "port": 3306,
		"username": "root", "password": "pw", "via_agent_id": a.ID,
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("create connection code=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)

	rec = post(t, r, "/api/query", map[string]any{
		"conn_id": created.Connection.ID,
		"sql":     "SELECT 1",
	}, tok)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d body=%s, want 502", rec.Code, rec.Body.String())
	}
}

func TestExecutorForQuery_RejectsCrossUserAgent(t *testing.T) {
	otherAgentID := int64(42)
	reg := agent.NewRegistry()
	reg.Register(&agent.Conn{
		AgentID: agent.AgentIDString(otherAgentID),
		UserID:  999,
	})

	_, err := executorForQuery(Deps{AgentRegistry: reg}, &connSession{
		Conn: store.Connection{
			ID:         1,
			UserID:     123,
			Host:       "127.0.0.1",
			Port:       3306,
			Username:   "root",
			ViaAgentID: &otherAgentID,
		},
		Password: "pw",
	}, "")
	if !errors.Is(err, agent.ErrAgentOffline) {
		t.Fatalf("err = %v, want ErrAgentOffline", err)
	}
}

func TestQuery_ViaAgentWebSocketIntegration(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	baseURL := srv.URL
	tok := registerAndLoginURL(t, baseURL, "alice", "supersecret123")

	agentRec := postURL(t, baseURL, "/api/auth/agents", map[string]any{"name": "windows"}, tok)
	var createdAgent struct {
		Agent struct{ ID int64 } `json:"agent"`
		Token string             `json:"token"`
	}
	_ = json.NewDecoder(agentRec.Body).Decode(&createdAgent)
	connRec := postURL(t, baseURL, "/api/connections", map[string]any{
		"name": "via-agent", "host": "127.0.0.1", "port": 3306,
		"username": "root", "password": "pw", "via_agent_id": createdAgent.Agent.ID,
	}, tok)
	var createdConn struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(connRec.Body).Decode(&createdConn)

	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/agent"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(websocket.StatusNormalClosure, "") })
	if err := wsjson.Write(ctx, c, agent.Envelope{
		Type: agent.TypeHello,
		Payload: agent.Hello{
			Token: createdAgent.Token, AgentVersion: "test", OS: "windows", Arch: "amd64", Hostname: "test-host",
		},
	}); err != nil {
		t.Fatal(err)
	}
	var ack agent.Envelope
	if err := wsjson.Read(ctx, c, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.Type != agent.TypeHelloAck {
		t.Fatalf("ack type = %q", ack.Type)
	}

	queryDone := make(chan struct{})
	go func() {
		defer close(queryDone)
		for {
			var env agent.Envelope
			if err := wsjson.Read(ctx, c, &env); err != nil {
				return
			}
			if env.Type != agent.TypeQueryRequest {
				continue
			}
			var req agent.QueryRequest
			if err := remarshalPayload(env.Payload, &req); err != nil {
				t.Errorf("query request payload: %v", err)
				return
			}
			_ = wsjson.Write(ctx, c, agent.Envelope{Type: agent.TypeQueryMeta, Payload: agent.QueryMeta{
				RequestID: req.RequestID,
				Columns:   []agent.ColInfo{{Name: "v", Type: "VARCHAR"}},
			}})
			_ = wsjson.Write(ctx, c, agent.Envelope{Type: agent.TypeQueryRows, Payload: agent.QueryRows{
				RequestID: req.RequestID,
				Rows:      [][]any{{"8.0.46"}},
			}})
			_ = wsjson.Write(ctx, c, agent.Envelope{Type: agent.TypeQueryDone, Payload: agent.QueryDone{
				RequestID:  req.RequestID,
				RowCount:   1,
				DurationMs: 3,
			}})
			return
		}
	}()

	queryRec := postURL(t, baseURL, "/api/query", map[string]any{
		"conn_id": createdConn.Connection.ID,
		"sql":     "SELECT version() AS v",
	}, tok)
	if queryRec.Code != http.StatusOK {
		t.Fatalf("query code=%d body=%s", queryRec.Code, queryRec.Body.String())
	}
	var body struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	_ = json.NewDecoder(queryRec.Body).Decode(&body)
	if len(body.Columns) != 1 || body.Columns[0] != "v" || len(body.Rows) != 1 || body.Rows[0][0] != "8.0.46" {
		t.Fatalf("body = %+v", body)
	}
	<-queryDone
}

func TestQueryStatusForError_DeadlineIs408(t *testing.T) {
	if got := queryStatusForError(context.DeadlineExceeded); got != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want 408", got)
	}
}
