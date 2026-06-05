package store

import (
	"testing"
)

func TestAIAuditWriteAndTransition(t *testing.T) {
	s := setupUsers(t)
	u, _ := s.CreateUser("alice", "longpassword1")

	id, err := s.WriteAIAudit(AIAuditRow{
		UserID: u.ID, ConnectionID: 1, Database: "db1", Table: "t1",
		Operation: "INSERT", SQL: "INSERT INTO t1 VALUES (1)",
		Status: "proposed", ExplainSummary: `{"rows":1}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected id")
	}

	n := int64(3)
	if err := s.UpdateAIAuditStatus(id, "executed", &n, ""); err != nil {
		t.Fatal(err)
	}

	rows, err := s.RecentAIAudit(u.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Status != "executed" {
		t.Fatalf("status=%s", rows[0].Status)
	}
	if rows[0].RowsAffected == nil || *rows[0].RowsAffected != 3 {
		t.Fatalf("rows_affected mismatch: %v", rows[0].RowsAffected)
	}
}

func TestAIAuditUserIsolation(t *testing.T) {
	s := setupUsers(t)
	a, _ := s.CreateUser("alice", "longpassword1")
	b, _ := s.CreateUser("bob", "longpassword2")
	_, _ = s.WriteAIAudit(AIAuditRow{UserID: a.ID, ConnectionID: 1, Database: "d", Table: "t", Operation: "INSERT", SQL: "x", Status: "proposed"})
	rows, _ := s.RecentAIAudit(b.ID, 10)
	if len(rows) != 0 {
		t.Fatalf("bob sees alice's row(s)")
	}
}

func TestRecentAIAuditLimitDefaults(t *testing.T) {
	s := setupUsers(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	// Seed 3 rows
	for i := 0; i < 3; i++ {
		_, _ = s.WriteAIAudit(AIAuditRow{
			UserID: u.ID, ConnectionID: 1, Database: "d", Table: "t",
			Operation: "INSERT", SQL: "x", Status: "proposed",
		})
	}
	// limit=0 → default 50, returns all 3
	rows, _ := s.RecentAIAudit(u.ID, 0)
	if len(rows) != 3 {
		t.Fatalf("limit=0 want 3, got %d", len(rows))
	}
	// limit=2 → returns 2
	rows, _ = s.RecentAIAudit(u.ID, 2)
	if len(rows) != 2 {
		t.Fatalf("limit=2 want 2, got %d", len(rows))
	}
	// limit=600 → capped at 500 (still only 3 rows in table though)
	rows, _ = s.RecentAIAudit(u.ID, 600)
	if len(rows) != 3 {
		t.Fatalf("limit=600 want 3 (capped at 500), got %d", len(rows))
	}
}
