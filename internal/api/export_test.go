package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestExport_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := get(t, r, "/api/db/1/databases/x/tables/t/export?format=csv", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestExport_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	rec := get(t, r, "/api/db/999/databases/x/tables/t/export?format=csv", tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestExport_BadFormat(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedDMLConn(t, r, tok)
	rec := get(t, r, "/api/db/"+itoa(connID)+"/databases/x/tables/t/export?format=zzz", tok)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "format must be") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
