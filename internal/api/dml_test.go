package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func patchJSON(t *testing.T, h http.Handler, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func deleteJSON(t *testing.T, h http.Handler, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodDelete, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPatchRow_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := patchJSON(t, r, "/api/db/1/databases/x/tables/t/rows",
		map[string]any{"pk_values": map[string]any{"id": 1}, "column": "name", "new_value": "x"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestPatchRow_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := patchJSON(t, r, "/api/db/999/databases/x/tables/t/rows",
		map[string]any{"pk_values": map[string]any{"id": 1}, "column": "name", "new_value": "x"}, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func seedDMLConn(t *testing.T, r http.Handler, tok string) int64 {
	t.Helper()
	rec := post(t, r, "/api/connections", map[string]any{
		"name": "c", "host": "h", "port": 3306, "username": "u", "password": "p",
	}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("create conn: %d %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Connection struct{ ID int64 } `json:"connection"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&got)
	return got.Connection.ID
}

func TestPatchRow_ColumnRequired(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedDMLConn(t, r, tok)
	rec := patchJSON(t, r, "/api/db/"+itoa(connID)+"/databases/x/tables/t/rows",
		map[string]any{"pk_values": map[string]any{"id": 1}, "new_value": "x"}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestInsertRow_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := post(t, r, "/api/db/1/databases/x/tables/t/rows",
		map[string]any{"values": map[string]any{"name": "x"}}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestInsertRow_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := post(t, r, "/api/db/999/databases/x/tables/t/rows",
		map[string]any{"values": map[string]any{"name": "x"}}, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestInsertRow_EmptyValues(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedDMLConn(t, r, tok)
	rec := post(t, r, "/api/db/"+itoa(connID)+"/databases/x/tables/t/rows",
		map[string]any{"values": map[string]any{}}, tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestDeleteRow_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := deleteJSON(t, r, "/api/db/1/databases/x/tables/t/rows",
		map[string]any{"pk_values": map[string]any{"id": 1}}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestDeleteRow_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := deleteJSON(t, r, "/api/db/999/databases/x/tables/t/rows",
		map[string]any{"pk_values": map[string]any{"id": 1}}, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}
