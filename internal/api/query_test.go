package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

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

func TestQueryStatusForError_DeadlineIs408(t *testing.T) {
	if got := queryStatusForError(context.DeadlineExceeded); got != http.StatusRequestTimeout {
		t.Fatalf("status = %d, want 408", got)
	}
}
