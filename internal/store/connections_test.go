package store

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/conray/dataseai/internal/crypto"
)

func newCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c, err := crypto.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func setupConnections(t *testing.T) (*Store, User, *crypto.Cipher) {
	t.Helper()
	s := setupUsers(t)
	u, err := s.CreateUser("alice", "supersecret123")
	if err != nil {
		t.Fatal(err)
	}
	return s, u, newCipher(t)
}

func TestCreateConnection_PersistsAndDoesNotStorePlaintext(t *testing.T) {
	s, u, c := setupConnections(t)
	conn, err := s.CreateConnection(c, u.ID, ConnectionInput{
		Name: "prod", Host: "db.example.com", Port: 3306,
		Username: "app", Password: "shhh!", TLS: "preferred",
	})
	if err != nil {
		t.Fatal(err)
	}
	if conn.ID == 0 || conn.Name != "prod" {
		t.Fatalf("conn = %+v", conn)
	}
	var enc []byte
	if err := s.DB.QueryRow("SELECT password_enc FROM connections WHERE id=?", conn.ID).Scan(&enc); err != nil {
		t.Fatal(err)
	}
	if string(enc) == "shhh!" || len(enc) == 0 {
		t.Fatalf("password stored unencrypted: %q", enc)
	}
}

func TestCreateConnection_DuplicateName(t *testing.T) {
	s, u, c := setupConnections(t)
	if _, err := s.CreateConnection(c, u.ID, ConnectionInput{Name: "prod", Host: "h", Port: 3306, Username: "u", Password: "p"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateConnection(c, u.ID, ConnectionInput{Name: "prod", Host: "h2", Port: 3306, Username: "u2", Password: "p2"})
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("want ErrDuplicate, got %v", err)
	}
}

func TestGetDecryptedPassword(t *testing.T) {
	s, u, c := setupConnections(t)
	in := ConnectionInput{Name: "prod", Host: "h", Port: 3306, Username: "u", Password: "the-real-pw"}
	conn, _ := s.CreateConnection(c, u.ID, in)
	pw, err := s.GetConnectionPassword(c, u.ID, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pw != "the-real-pw" {
		t.Fatalf("decrypted = %q, want the-real-pw", pw)
	}
}

func TestConnection_RoundTripsViaAgentID(t *testing.T) {
	s, u, c := setupConnections(t)
	agent, _, err := s.CreateAgent(u.ID, "windows")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := s.CreateConnection(c, u.ID, ConnectionInput{
		Name: "local-mysql", Host: "127.0.0.1", Port: 3306,
		Username: "root", Password: "pw", ViaAgentID: &agent.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if conn.ViaAgentID == nil || *conn.ViaAgentID != agent.ID {
		t.Fatalf("created ViaAgentID = %v, want %d", conn.ViaAgentID, agent.ID)
	}
	got, err := s.GetConnection(u.ID, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ViaAgentID == nil || *got.ViaAgentID != agent.ID {
		t.Fatalf("loaded ViaAgentID = %v, want %d", got.ViaAgentID, agent.ID)
	}
}

func TestGetConnectionPassword_WrongUser(t *testing.T) {
	s, u, c := setupConnections(t)
	in := ConnectionInput{Name: "prod", Host: "h", Port: 3306, Username: "u", Password: "x"}
	conn, _ := s.CreateConnection(c, u.ID, in)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	_, err := s.GetConnectionPassword(c, bob.ID, conn.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for cross-user access, got %v", err)
	}
}

func TestListConnections_ScopedToUser(t *testing.T) {
	s, u, c := setupConnections(t)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	_, _ = s.CreateConnection(c, u.ID, ConnectionInput{Name: "a-prod", Host: "h", Port: 3306, Username: "u", Password: "p"})
	_, _ = s.CreateConnection(c, u.ID, ConnectionInput{Name: "a-dev", Host: "h", Port: 3306, Username: "u", Password: "p"})
	_, _ = s.CreateConnection(c, bob.ID, ConnectionInput{Name: "b-prod", Host: "h", Port: 3306, Username: "u", Password: "p"})

	list, err := s.ListConnections(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("alice should see 2, got %d", len(list))
	}
	for _, c := range list {
		if c.UserID != u.ID {
			t.Fatalf("leaked connection from user %d into alice's list", c.UserID)
		}
	}
}

func TestUpdateConnection_PreservesPasswordWhenEmpty(t *testing.T) {
	s, u, c := setupConnections(t)
	conn, _ := s.CreateConnection(c, u.ID, ConnectionInput{Name: "prod", Host: "h", Port: 3306, Username: "u", Password: "orig-pw"})
	upd, err := s.UpdateConnection(c, u.ID, conn.ID, ConnectionInput{Name: "prod-renamed", Host: "h2", Port: 3307, Username: "u2", Password: ""})
	if err != nil {
		t.Fatal(err)
	}
	if upd.Name != "prod-renamed" || upd.Host != "h2" || upd.Port != 3307 {
		t.Fatalf("update did not persist: %+v", upd)
	}
	pw, _ := s.GetConnectionPassword(c, u.ID, conn.ID)
	if pw != "orig-pw" {
		t.Fatalf("password was clobbered: %q", pw)
	}
}

func TestDeleteConnection_ScopedToUser(t *testing.T) {
	s, u, c := setupConnections(t)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	conn, _ := s.CreateConnection(c, bob.ID, ConnectionInput{Name: "b-prod", Host: "h", Port: 3306, Username: "u", Password: "p"})
	err := s.DeleteConnection(u.ID, conn.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("alice should not see bob's conn, got %v", err)
	}
	if err := s.DeleteConnection(bob.ID, conn.ID); err != nil {
		t.Fatal(err)
	}
}
