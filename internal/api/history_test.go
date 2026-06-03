package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func seedHistory(t *testing.T, r http.Handler, tok string) int64 {
	t.Helper()
	rec := post(t, r, "/api/connections", map[string]any{"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p"}, tok)
	var created struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	rec = post(t, r, "/api/query", map[string]any{"conn_id": created.Connection.ID, "sql": "SELECT 1"}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed query failed: %d %s", rec.Code, rec.Body.String())
	}
	return created.Connection.ID
}

func TestListHistory(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	_ = seedHistory(t, r, tok)
	rec := get(t, r, "/api/history", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var body struct {
		History []map[string]any `json:"history"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.History) != 1 {
		t.Fatalf("len = %d", len(body.History))
	}
}

func TestDeleteHistoryEntry(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	_ = seedHistory(t, r, tok)
	rec := get(t, r, "/api/history", tok)
	var body struct {
		History []struct{ ID int64 } `json:"history"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	id := body.History[0].ID
	rec = delete_(t, r, "/api/history/"+itoa(id), tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete code = %d", rec.Code)
	}
	rec = get(t, r, "/api/history", tok)
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.History) != 0 {
		t.Fatalf("len after delete = %d", len(body.History))
	}
}

func TestClearHistory(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedHistory(t, r, tok)
	_ = post(t, r, "/api/query", map[string]any{"conn_id": connID, "sql": "SELECT 1"}, tok)
	_ = post(t, r, "/api/query", map[string]any{"conn_id": connID, "sql": "SELECT 1"}, tok)
	rec := delete_(t, r, "/api/history", tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("clear code = %d", rec.Code)
	}
	rec = get(t, r, "/api/history", tok)
	var body struct {
		History []map[string]any `json:"history"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if len(body.History) != 0 {
		t.Fatalf("len after clear = %d", len(body.History))
	}
}
