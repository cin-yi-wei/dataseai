package store

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func setupUsers(t *testing.T) *Store {
	t.Helper()
	db := openMem(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &Store{DB: db}
}

func TestCreateUser_StoresHashedPassword(t *testing.T) {
	s := setupUsers(t)
	u, err := s.CreateUser("alice", "supersecret123")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID == 0 || u.Username != "alice" {
		t.Fatalf("user = %+v", u)
	}
	var hash string
	if err := s.DB.QueryRow("SELECT password_hash FROM users WHERE id=?", u.ID).Scan(&hash); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if hash == "supersecret123" || hash == "" {
		t.Fatalf("password not hashed: %q", hash)
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	s := setupUsers(t)
	if _, err := s.CreateUser("alice", "supersecret123"); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateUser("alice", "anotherpassword")
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestVerifyPassword_HappyPath(t *testing.T) {
	s := setupUsers(t)
	u, _ := s.CreateUser("alice", "supersecret123")
	got, err := s.VerifyPassword("alice", "supersecret123")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID {
		t.Fatalf("id mismatch")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	s := setupUsers(t)
	_, _ = s.CreateUser("alice", "supersecret123")
	_, err := s.VerifyPassword("alice", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestVerifyPassword_UnknownUser(t *testing.T) {
	s := setupUsers(t)
	_, err := s.VerifyPassword("ghost", "x")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestVerifyPassword_DummyHashIsValid(t *testing.T) {
	// Sanity-check the baked-in dummy hash is a real bcrypt hash
	// (not malformed). If the const above ever gets clobbered,
	// every unknown-user request would fail-fast with an error instead
	// of returning ErrInvalidCredentials, breaking the timing-leak fix.
	err := bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte("never-matches-anything"))
	if err != nil {
		t.Fatalf("dummy hash is broken: %v", err)
	}
}

func TestAIWritesEnabledRoundTrip(t *testing.T) {
	s := setupUsers(t)
	u, err := s.CreateUser("alice", "longpassword1")
	if err != nil {
		t.Fatal(err)
	}

	enabled, err := s.GetAIWritesEnabled(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("expected default false")
	}

	if err := s.SetAIWritesEnabled(u.ID, true); err != nil {
		t.Fatal(err)
	}
	enabled, _ = s.GetAIWritesEnabled(u.ID)
	if !enabled {
		t.Fatal("expected true after set")
	}

	if err := s.SetAIWritesEnabled(u.ID, false); err != nil {
		t.Fatal(err)
	}
	enabled, _ = s.GetAIWritesEnabled(u.ID)
	if enabled {
		t.Fatal("expected false after clear")
	}
}
