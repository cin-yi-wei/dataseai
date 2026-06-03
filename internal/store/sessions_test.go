package store

import (
	"errors"
	"testing"
	"time"
)

func setupSessions(t *testing.T) (*Store, User) {
	t.Helper()
	s := setupUsers(t)
	u, err := s.CreateUser("alice", "supersecret123")
	if err != nil {
		t.Fatal(err)
	}
	return s, u
}

func TestCreateSession_ReturnsTokenAndPersists(t *testing.T) {
	s, u := setupSessions(t)
	sess, err := s.CreateSession(u.ID, "Mozilla/5.0", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Token) != 64 {
		t.Fatalf("token length = %d, want 64", len(sess.Token))
	}
	if sess.UserID != u.ID {
		t.Fatalf("user_id mismatch")
	}
	got, err := s.GetSession(sess.Token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserID != u.ID {
		t.Fatalf("get user_id mismatch")
	}
}

func TestGetSession_Expired(t *testing.T) {
	s, u := setupSessions(t)
	sess, _ := s.CreateSession(u.ID, "ua", -1*time.Hour)
	_, err := s.GetSession(sess.Token)
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("want ErrSessionExpired, got %v", err)
	}
}

func TestDeleteSession(t *testing.T) {
	s, u := setupSessions(t)
	sess, _ := s.CreateSession(u.ID, "ua", time.Hour)
	if err := s.DeleteSession(sess.Token); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetSession(sess.Token)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}

func TestListSessionsByUser(t *testing.T) {
	s, u := setupSessions(t)
	_, _ = s.CreateSession(u.ID, "laptop", time.Hour)
	_, _ = s.CreateSession(u.ID, "phone", time.Hour)
	list, err := s.ListSessionsByUser(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
}

func TestRefreshSession_UpdatesLastUsed(t *testing.T) {
	s, u := setupSessions(t)
	sess, _ := s.CreateSession(u.ID, "ua", time.Hour)
	time.Sleep(1100 * time.Millisecond)
	if err := s.RefreshSession(sess.Token, time.Hour); err != nil {
		t.Fatal(err)
	}
	var lastUsed time.Time
	_ = s.DB.QueryRow("SELECT last_used_at FROM sessions WHERE token=?", sess.Token).Scan(&lastUsed)
	if time.Since(lastUsed) > 500*time.Millisecond {
		t.Fatalf("last_used_at not refreshed: %v", lastUsed)
	}
}

func TestDeleteUserSessionsExcept(t *testing.T) {
	s, u := setupSessions(t)
	keep, _ := s.CreateSession(u.ID, "keep", time.Hour)
	_, _ = s.CreateSession(u.ID, "drop1", time.Hour)
	_, _ = s.CreateSession(u.ID, "drop2", time.Hour)
	if err := s.DeleteUserSessionsExcept(u.ID, keep.Token); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListSessionsByUser(u.ID)
	if len(list) != 1 || list[0].Token != keep.Token {
		t.Fatalf("kept = %+v", list)
	}
}
