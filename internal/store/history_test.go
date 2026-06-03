package store

import (
	"testing"
)

func setupHistory(t *testing.T) (*Store, User, int64) {
	t.Helper()
	s, u, c := setupConnections(t)
	conn, err := s.CreateConnection(c, u.ID, ConnectionInput{Name: "x", Host: "h", Port: 3306, Username: "u", Password: "p"})
	if err != nil {
		t.Fatal(err)
	}
	return s, u, conn.ID
}

func TestAddHistory_Persists(t *testing.T) {
	s, u, connID := setupHistory(t)
	err := s.AddHistory(HistoryInput{
		UserID: u.ID, ConnectionID: connID, DatabaseName: "demo",
		SQLText: "SELECT 1", DurationMs: 5, RowsAffected: 0, Source: "user",
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListHistory(u.ID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d entries", len(list))
	}
	if list[0].SQLText != "SELECT 1" || list[0].DurationMs != 5 {
		t.Fatalf("entry = %+v", list[0])
	}
}

func TestListHistory_ScopedToUser(t *testing.T) {
	s, alice, aliceConn := setupHistory(t)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	bobConn, _ := s.CreateConnection(newCipher(t), bob.ID, ConnectionInput{Name: "b", Host: "h", Port: 3306, Username: "u", Password: "p"})
	_ = s.AddHistory(HistoryInput{UserID: alice.ID, ConnectionID: aliceConn, SQLText: "a-query"})
	_ = s.AddHistory(HistoryInput{UserID: bob.ID, ConnectionID: bobConn.ID, SQLText: "b-query"})
	list, _ := s.ListHistory(alice.ID, 50, 0)
	if len(list) != 1 || list[0].SQLText != "a-query" {
		t.Fatalf("alice sees %+v", list)
	}
}

func TestDeleteHistoryEntry(t *testing.T) {
	s, u, connID := setupHistory(t)
	_ = s.AddHistory(HistoryInput{UserID: u.ID, ConnectionID: connID, SQLText: "x"})
	list, _ := s.ListHistory(u.ID, 50, 0)
	if err := s.DeleteHistoryEntry(u.ID, list[0].ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.ListHistory(u.ID, 50, 0)
	if len(list) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(list))
	}
}

func TestDeleteHistoryEntry_CrossUserBlocked(t *testing.T) {
	s, alice, aliceConn := setupHistory(t)
	bob, _ := s.CreateUser("bob", "anothersecret456")
	_ = s.AddHistory(HistoryInput{UserID: alice.ID, ConnectionID: aliceConn, SQLText: "a"})
	list, _ := s.ListHistory(alice.ID, 50, 0)
	// bob tries to delete alice's entry
	if err := s.DeleteHistoryEntry(bob.ID, list[0].ID); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestClearHistory(t *testing.T) {
	s, u, connID := setupHistory(t)
	for i := 0; i < 5; i++ {
		_ = s.AddHistory(HistoryInput{UserID: u.ID, ConnectionID: connID, SQLText: "q"})
	}
	if err := s.ClearHistory(u.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListHistory(u.ID, 50, 0)
	if len(list) != 0 {
		t.Fatalf("expected 0 after clear, got %d", len(list))
	}
}

func TestAddHistory_PrunesOverCap(t *testing.T) {
	s, u, connID := setupHistory(t)
	for i := 0; i < 12; i++ {
		_ = s.AddHistoryWithCap(HistoryInput{UserID: u.ID, ConnectionID: connID, SQLText: "q"}, 10)
	}
	list, _ := s.ListHistory(u.ID, 50, 0)
	if len(list) != 10 {
		t.Fatalf("expected 10 entries after prune cap=10, got %d", len(list))
	}
}
