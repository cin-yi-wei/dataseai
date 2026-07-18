package store

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	return &Store{DB: db}
}

func TestResetCode_HappyPath(t *testing.T) {
	s := newStore(t)
	u, err := s.CreateUser("adam", "oldpass123")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEmail(u.ID, "adam@example.com"); err != nil {
		t.Fatal(err)
	}

	// lookup by username and by email both resolve to the same user + email
	for _, id := range []string{"adam", "ADAM@example.com"} {
		uid, email, ok := s.LookupForReset(id)
		if !ok || uid != u.ID || email != "adam@example.com" {
			t.Fatalf("lookup(%q) = %d,%q,%v", id, uid, email, ok)
		}
	}

	now := time.Unix(1_700_000_000, 0)
	if err := s.CreateResetCode(u.ID, "hash123", now, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	// valid within ttl
	if err := s.UseResetCode(u.ID, "hash123", now.Add(time.Minute)); err != nil {
		t.Fatalf("use valid code: %v", err)
	}
	// second use fails (single-use)
	if err := s.UseResetCode(u.ID, "hash123", now.Add(2*time.Minute)); err != ErrNotFound {
		t.Fatalf("reuse should fail, got %v", err)
	}
}

func TestResetCode_Expired(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("adam", "oldpass123")
	now := time.Unix(1_700_000_000, 0)
	_ = s.CreateResetCode(u.ID, "h", now, 10*time.Minute)
	if err := s.UseResetCode(u.ID, "h", now.Add(11*time.Minute)); err != ErrNotFound {
		t.Fatalf("expired code should fail, got %v", err)
	}
}

func TestLookupForReset_NoEmailOrUser(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("adam", "oldpass123") // no email set
	if _, _, ok := s.LookupForReset("adam"); ok {
		t.Fatal("user with no email should not be resettable")
	}
	if _, _, ok := s.LookupForReset("ghost"); ok {
		t.Fatal("unknown identifier should not resolve")
	}
	_ = u
}
