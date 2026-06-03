package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func csvMultipart(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("file", "x.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("a,b\n1,2\n")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return body, mw.FormDataContentType()
}

func TestImport_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	body, contentType := csvMultipart(t)
	req := httptest.NewRequest(http.MethodPost, "/api/db/1/databases/x/tables/t/import", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestImport_UnknownConn(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	body, contentType := csvMultipart(t)
	req := httptest.NewRequest(http.MethodPost, "/api/db/999/databases/x/tables/t/import", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}
