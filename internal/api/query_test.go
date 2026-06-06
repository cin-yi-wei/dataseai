package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func getURL(t *testing.T, baseURL, path string, token string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
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

func connectTestAgent(t *testing.T, baseURL, token string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/agent"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, c, agent.Envelope{
		Type: agent.TypeHello,
		Payload: agent.Hello{
			Token: token, AgentVersion: "test", OS: "windows", Arch: "amd64", Hostname: "test-host",
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
	return c
}

type agentQueryReply struct {
	columns []agent.ColInfo
	rows    [][]any
	err     string
}

func serveAgentQueries(t *testing.T, c *websocket.Conn, replies map[string]agentQueryReply) {
	t.Helper()
	ctx := context.Background()
	go func() {
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
			var reply *agentQueryReply
			longestNeedle := ""
			for needle, r := range replies {
				if strings.Contains(req.SQL, needle) && len(needle) > len(longestNeedle) {
					copy := r
					reply = &copy
					longestNeedle = needle
				}
			}
			if reply == nil {
				_ = wsjson.Write(ctx, c, agent.Envelope{Type: agent.TypeQueryError, Payload: agent.QueryError{
					RequestID: req.RequestID,
					Error:     "unexpected SQL: " + req.SQL,
				}})
				continue
			}
			if reply.err != "" {
				_ = wsjson.Write(ctx, c, agent.Envelope{Type: agent.TypeQueryError, Payload: agent.QueryError{
					RequestID: req.RequestID,
					Error:     reply.err,
				}})
				continue
			}
			_ = wsjson.Write(ctx, c, agent.Envelope{Type: agent.TypeQueryMeta, Payload: agent.QueryMeta{
				RequestID: req.RequestID,
				Columns:   reply.columns,
			}})
			_ = wsjson.Write(ctx, c, agent.Envelope{Type: agent.TypeQueryRows, Payload: agent.QueryRows{
				RequestID: req.RequestID,
				Rows:      reply.rows,
			}})
			_ = wsjson.Write(ctx, c, agent.Envelope{Type: agent.TypeQueryDone, Payload: agent.QueryDone{
				RequestID:  req.RequestID,
				RowCount:   len(reply.rows),
				DurationMs: 1,
			}})
		}
	}()
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

func TestDBReadEndpoints_ViaAgentWebSocketIntegration(t *testing.T) {
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

	c := connectTestAgent(t, baseURL, createdAgent.Token)
	t.Cleanup(func() { _ = c.Close(websocket.StatusNormalClosure, "") })
	serveAgentQueries(t, c, map[string]agentQueryReply{
		"SHOW DATABASES": {
			columns: []agent.ColInfo{{Name: "Database", Type: "VARCHAR"}},
			rows:    [][]any{{"appdb"}},
		},
		"FROM information_schema.tables": {
			columns: []agent.ColInfo{{Name: "table_name", Type: "VARCHAR"}, {Name: "table_rows", Type: "BIGINT"}, {Name: "size_mb", Type: "BIGINT"}},
			rows:    [][]any{{"users", 12, 1}},
		},
		"SELECT COUNT(*) FROM `appdb`.`users`": {
			columns: []agent.ColInfo{{Name: "COUNT(*)", Type: "BIGINT"}},
			rows:    [][]any{{2}},
		},
		"SELECT * FROM `appdb`.`users`": {
			columns: []agent.ColInfo{{Name: "id", Type: "BIGINT"}, {Name: "name", Type: "VARCHAR"}},
			rows:    [][]any{{1, "alice"}, {2, "bob"}},
		},
		"SELECT COUNT(*) FROM `appdb`.`users` WHERE `name` LIKE '%ali%'": {
			columns: []agent.ColInfo{{Name: "COUNT(*)", Type: "BIGINT"}},
			rows:    [][]any{{1}},
		},
		"SELECT * FROM `appdb`.`users` WHERE `name` LIKE '%ali%'": {
			columns: []agent.ColInfo{{Name: "id", Type: "BIGINT"}, {Name: "name", Type: "VARCHAR"}},
			rows:    [][]any{{1, "alice"}},
		},
		"ORDER BY table_name, ordinal_position": {
			columns: []agent.ColInfo{{Name: "table_name", Type: "VARCHAR"}, {Name: "column_name", Type: "VARCHAR"}},
			rows:    [][]any{{"users", "id"}, {"users", "name"}},
		},
		"AND table_name = 'users'": {
			columns: []agent.ColInfo{
				{Name: "column_name", Type: "VARCHAR"},
				{Name: "column_type", Type: "VARCHAR"},
				{Name: "is_nullable", Type: "VARCHAR"},
				{Name: "column_default", Type: "VARCHAR"},
				{Name: "extra", Type: "VARCHAR"},
				{Name: "column_comment", Type: "VARCHAR"},
				{Name: "column_key", Type: "VARCHAR"},
			},
			rows: [][]any{{"id", "bigint", "NO", "", "auto_increment", "", "PRI"}, {"name", "varchar(255)", "YES", "", "", "", ""}},
		},
		"SHOW CREATE TABLE `appdb`.`users`": {
			columns: []agent.ColInfo{{Name: "Table", Type: "VARCHAR"}, {Name: "Create Table", Type: "VARCHAR"}},
			rows:    [][]any{{"users", "CREATE TABLE `users` (`id` bigint primary key)"}},
		},
		"FROM information_schema.statistics": {
			columns: []agent.ColInfo{{Name: "index_name", Type: "VARCHAR"}, {Name: "column_name", Type: "VARCHAR"}, {Name: "non_unique", Type: "INT"}, {Name: "index_type", Type: "VARCHAR"}},
			rows:    [][]any{{"PRIMARY", "id", 0, "BTREE"}},
		},
		"FROM information_schema.key_column_usage": {
			columns: []agent.ColInfo{{Name: "constraint_name", Type: "VARCHAR"}, {Name: "column_name", Type: "VARCHAR"}, {Name: "referenced_table_name", Type: "VARCHAR"}, {Name: "referenced_column_name", Type: "VARCHAR"}, {Name: "delete_rule", Type: "VARCHAR"}, {Name: "update_rule", Type: "VARCHAR"}},
			rows:    [][]any{{"fk_users_role", "role_id", "roles", "id", "CASCADE", "RESTRICT"}},
		},
	})

	dbRec := getURL(t, baseURL, "/api/db/"+itoa(createdConn.Connection.ID)+"/databases", tok)
	if dbRec.Code != http.StatusOK {
		t.Fatalf("databases code=%d body=%s", dbRec.Code, dbRec.Body.String())
	}
	var dbBody struct {
		Databases []string `json:"databases"`
	}
	_ = json.NewDecoder(dbRec.Body).Decode(&dbBody)
	if len(dbBody.Databases) != 1 || dbBody.Databases[0] != "appdb" {
		t.Fatalf("databases = %+v", dbBody.Databases)
	}

	tablesRec := getURL(t, baseURL, "/api/db/"+itoa(createdConn.Connection.ID)+"/databases/appdb/tables", tok)
	if tablesRec.Code != http.StatusOK {
		t.Fatalf("tables code=%d body=%s", tablesRec.Code, tablesRec.Body.String())
	}
	var tablesBody struct {
		Tables []struct {
			Name string `json:"name"`
		} `json:"tables"`
	}
	_ = json.NewDecoder(tablesRec.Body).Decode(&tablesBody)
	if len(tablesBody.Tables) != 1 || tablesBody.Tables[0].Name != "users" {
		t.Fatalf("tables = %+v", tablesBody.Tables)
	}

	rowsRec := getURL(t, baseURL, "/api/db/"+itoa(createdConn.Connection.ID)+"/databases/appdb/tables/users/data?per_page=2", tok)
	if rowsRec.Code != http.StatusOK {
		t.Fatalf("rows code=%d body=%s", rowsRec.Code, rowsRec.Body.String())
	}
	var rowsBody struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
		Total   int64    `json:"total"`
	}
	_ = json.NewDecoder(rowsRec.Body).Decode(&rowsBody)
	if rowsBody.Total != 2 || len(rowsBody.Rows) != 2 || rowsBody.Columns[1] != "name" {
		t.Fatalf("rows = %+v", rowsBody)
	}

	filterJSON := `[{"column":"name","operator":"Contains","value":"ali"}]`
	filteredRec := getURL(t, baseURL, "/api/db/"+itoa(createdConn.Connection.ID)+"/databases/appdb/tables/users/data?per_page=2&filters="+url.QueryEscape(filterJSON), tok)
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("filtered rows code=%d body=%s", filteredRec.Code, filteredRec.Body.String())
	}
	var filteredBody struct {
		Rows  [][]any `json:"rows"`
		Total int64   `json:"total"`
	}
	_ = json.NewDecoder(filteredRec.Body).Decode(&filteredBody)
	if filteredBody.Total != 1 || len(filteredBody.Rows) != 1 || filteredBody.Rows[0][1] != "alice" {
		t.Fatalf("filtered rows = %+v", filteredBody)
	}

	schemaRec := getURL(t, baseURL, "/api/db/"+itoa(createdConn.Connection.ID)+"/databases/appdb/schema", tok)
	if schemaRec.Code != http.StatusOK {
		t.Fatalf("schema code=%d body=%s", schemaRec.Code, schemaRec.Body.String())
	}
	var schemaBody struct {
		Tables map[string][]string `json:"tables"`
	}
	_ = json.NewDecoder(schemaRec.Body).Decode(&schemaBody)
	if got := schemaBody.Tables["users"]; len(got) != 2 || got[0] != "id" || got[1] != "name" {
		t.Fatalf("schema = %+v", schemaBody.Tables)
	}

	structureRec := getURL(t, baseURL, "/api/db/"+itoa(createdConn.Connection.ID)+"/databases/appdb/tables/users/structure", tok)
	if structureRec.Code != http.StatusOK {
		t.Fatalf("structure code=%d body=%s", structureRec.Code, structureRec.Body.String())
	}
	var structureBody struct {
		Columns []struct {
			Name     string `json:"name"`
			Nullable bool   `json:"nullable"`
			Key      string `json:"key"`
		} `json:"columns"`
		CreateSQL string `json:"create_sql"`
	}
	_ = json.NewDecoder(structureRec.Body).Decode(&structureBody)
	if len(structureBody.Columns) != 2 || structureBody.Columns[0].Name != "id" || !strings.Contains(structureBody.CreateSQL, "CREATE TABLE") {
		t.Fatalf("structure = %+v", structureBody)
	}

	indexRec := getURL(t, baseURL, "/api/db/"+itoa(createdConn.Connection.ID)+"/databases/appdb/tables/users/indexes", tok)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("indexes code=%d body=%s", indexRec.Code, indexRec.Body.String())
	}
	var indexBody struct {
		Indexes []struct {
			Name   string   `json:"name"`
			Unique bool     `json:"unique"`
			Cols   []string `json:"columns"`
		} `json:"indexes"`
	}
	_ = json.NewDecoder(indexRec.Body).Decode(&indexBody)
	if len(indexBody.Indexes) != 1 || indexBody.Indexes[0].Name != "PRIMARY" || !indexBody.Indexes[0].Unique {
		t.Fatalf("indexes = %+v", indexBody.Indexes)
	}

	fkRec := getURL(t, baseURL, "/api/db/"+itoa(createdConn.Connection.ID)+"/databases/appdb/tables/users/fks", tok)
	if fkRec.Code != http.StatusOK {
		t.Fatalf("fks code=%d body=%s", fkRec.Code, fkRec.Body.String())
	}
	var fkBody struct {
		FKs []struct {
			Name     string `json:"name"`
			RefTable string `json:"ref_table"`
		} `json:"fks"`
	}
	_ = json.NewDecoder(fkRec.Body).Decode(&fkBody)
	if len(fkBody.FKs) != 1 || fkBody.FKs[0].Name != "fk_users_role" || fkBody.FKs[0].RefTable != "roles" {
		t.Fatalf("fks = %+v", fkBody.FKs)
	}
}

func TestQueryStatusForError_DeadlineIs408(t *testing.T) {
	if got := queryStatusForError(context.DeadlineExceeded); got != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want 408", got)
	}
}
