package api

import (
	"database/sql"
	"net/http"
	"regexp"
	"testing"

	"github.com/conray/dataseai/internal/mail"
	"github.com/conray/dataseai/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

// newEmailResetRouter enables the email-code reset flow with a MockSender.
func newEmailResetRouter(t *testing.T) (http.Handler, *store.Store, *mail.MockSender) {
	t.Helper()
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{DB: db}
	m := &mail.MockSender{}
	h := NewRouter(Deps{Version: "test", Store: s, Registration: "open", ForgotPasswordEnabled: true, Mailer: m})
	return h, s, m
}

var sixDigit = regexp.MustCompile(`\b(\d{6})\b`)

func TestEmailReset_FullFlow(t *testing.T) {
	r, s, m := newEmailResetRouter(t)
	u := registerAndLogin(t, r, "adam", "oldpass123")
	_ = u
	uid, _, _ := s.LookupForReset("adam") // no email yet -> unusable
	if uid != 0 {
		t.Fatal("no email set yet, should not resolve")
	}
	// set email
	var au store.User
	au, _ = s.VerifyPassword("adam", "oldpass123")
	if err := s.SetEmail(au.ID, "adam@example.com"); err != nil {
		t.Fatal(err)
	}

	// step 1: request code (by email) -> 204, mock got a mail
	if rc := post(t, r, "/api/auth/forgot-password", map[string]string{"username": "adam@example.com"}, "").Code; rc != http.StatusNoContent {
		t.Fatalf("request code = %d", rc)
	}
	if len(m.Sent) != 1 || m.Sent[0].To != "adam@example.com" {
		t.Fatalf("mail not sent: %+v", m.Sent)
	}
	code := sixDigit.FindString(m.Sent[0].Body)
	if code == "" {
		t.Fatalf("no code in body: %q", m.Sent[0].Body)
	}

	// wrong code -> 400
	if rc := post(t, r, "/api/auth/reset-password", map[string]string{"username": "adam", "code": "000000", "new": "newpass456"}, "").Code; rc != http.StatusBadRequest {
		t.Fatalf("wrong code should 400, got %d", rc)
	}
	// correct code -> 204
	if rc := post(t, r, "/api/auth/reset-password", map[string]string{"username": "adam", "code": code, "new": "newpass456"}, "").Code; rc != http.StatusNoContent {
		t.Fatalf("valid code reset = %d", rc)
	}
	// old pw fails, new pw works
	if rc := post(t, r, "/api/auth/login", map[string]string{"username": "adam", "password": "oldpass123"}, "").Code; rc != http.StatusUnauthorized {
		t.Fatalf("old pw should fail, got %d", rc)
	}
	if rc := post(t, r, "/api/auth/login", map[string]string{"username": "adam", "password": "newpass456"}, "").Code; rc != http.StatusOK {
		t.Fatalf("new pw should log in, got %d", rc)
	}
	// reused code -> 400 (single-use)
	if rc := post(t, r, "/api/auth/reset-password", map[string]string{"username": "adam", "code": code, "new": "another789"}, "").Code; rc != http.StatusBadRequest {
		t.Fatalf("reused code should 400, got %d", rc)
	}
}

func TestEmailReset_UnknownAccount_StillNoContent(t *testing.T) {
	r, _, m := newEmailResetRouter(t)
	// no such user -> must still 204 (no enumeration) and send nothing
	if rc := post(t, r, "/api/auth/forgot-password", map[string]string{"username": "ghost@example.com"}, "").Code; rc != http.StatusNoContent {
		t.Fatalf("unknown = %d", rc)
	}
	if len(m.Sent) != 0 {
		t.Fatalf("should not send for unknown account: %+v", m.Sent)
	}
}

func TestAuthConfig_ReportsEmailReset(t *testing.T) {
	r, _, _ := newEmailResetRouter(t)
	rec := get(t, r, "/api/auth/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("config = %d", rec.Code)
	}
	if !regexp.MustCompile(`"email_reset":\s*true`).MatchString(rec.Body.String()) {
		t.Fatalf("email_reset not true: %s", rec.Body.String())
	}
}
