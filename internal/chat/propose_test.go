package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/conray/dataseai/internal/store"
)

func newTestChatStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return &store.Store{DB: db}
}

func setupTestSQLiteWithT(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TABLE t (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	return db
}

type fakeGateway struct {
	proposals []Proposal
	decision  Decision
}

func (g *fakeGateway) Propose(ctx context.Context, p Proposal) (Decision, error) {
	g.proposals = append(g.proposals, p)
	return g.decision, nil
}

func TestProposeWriteRejectsMultiStatement(t *testing.T) {
	s := newTestChatStore(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.SetAIWritesEnabled(u.ID, true)
	_ = s.UpsertAIPolicy(u.ID, 1, "db", "t", store.AIPolicy{Insert: true})

	g := &fakeGateway{decision: Decision{Accept: true}}
	out, _ := handleProposeWrite(context.Background(), ExecCtx{
		Store: s, Gateway: g, DB: nil, UserID: u.ID, ConnID: 1, DefaultDB: "db",
	}, map[string]any{
		"database":  "db",
		"table":     "t",
		"operation": "INSERT",
		"sql":       "INSERT INTO t VALUES (1); DELETE FROM t",
	})
	if !strings.Contains(out, "multi_statement") {
		t.Fatalf("got %s", out)
	}
}

func TestProposeWriteClassifyMismatch(t *testing.T) {
	s := newTestChatStore(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.SetAIWritesEnabled(u.ID, true)
	_ = s.UpsertAIPolicy(u.ID, 1, "db", "t", store.AIPolicy{Insert: true})

	g := &fakeGateway{decision: Decision{Accept: true}}
	out, _ := handleProposeWrite(context.Background(), ExecCtx{
		Store: s, Gateway: g, DB: nil, UserID: u.ID, ConnID: 1, DefaultDB: "db",
	}, map[string]any{
		"database":  "db",
		"table":     "t",
		"operation": "INSERT",
		"sql":       "DELETE FROM t WHERE id=1",
	})
	if !strings.Contains(out, "invalid_proposal") {
		t.Fatalf("got %s", out)
	}
}

func TestProposeWritePolicyDenied(t *testing.T) {
	s := newTestChatStore(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.SetAIWritesEnabled(u.ID, true)
	// No policy upserted → policy_denied

	g := &fakeGateway{decision: Decision{Accept: true}}
	out, _ := handleProposeWrite(context.Background(), ExecCtx{
		Store: s, Gateway: g, DB: nil, UserID: u.ID, ConnID: 1, DefaultDB: "db",
	}, map[string]any{
		"database":  "db",
		"table":     "t",
		"operation": "INSERT",
		"sql":       "INSERT INTO t VALUES (1)",
	})
	if !strings.Contains(out, "policy_denied") {
		t.Fatalf("got %s", out)
	}
	rows, _ := s.RecentAIAudit(u.ID, 10)
	if len(rows) != 1 || rows[0].Status != "denied" {
		t.Fatalf("expected one 'denied' audit, got %v", rows)
	}
}

func TestProposeWriteAcceptAndExecute(t *testing.T) {
	s := newTestChatStore(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.SetAIWritesEnabled(u.ID, true)
	_ = s.UpsertAIPolicy(u.ID, 1, "db", "t", store.AIPolicy{Insert: true})

	db := setupTestSQLiteWithT(t)
	g := &fakeGateway{decision: Decision{Accept: true}}

	out, _ := handleProposeWrite(context.Background(), ExecCtx{
		Store: s, Gateway: g, DB: db, UserID: u.ID, ConnID: 1, DefaultDB: "db",
	}, map[string]any{
		"database":  "db",
		"table":     "t",
		"operation": "INSERT",
		"sql":       "INSERT INTO t VALUES (42)",
	})

	var got map[string]any
	_ = json.Unmarshal([]byte(out), &got)
	if got["status"] != "executed" {
		t.Fatalf("got %v", got)
	}
	rows, _ := s.RecentAIAudit(u.ID, 10)
	if len(rows) != 1 || rows[0].Status != "executed" {
		t.Fatalf("expected 'executed' audit, got %+v", rows)
	}
	var cnt int
	if err := db.QueryRow("SELECT COUNT(*) FROM t WHERE id=42").Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("expected row inserted, got count=%d", cnt)
	}
}

func TestProposeWriteUserCancels(t *testing.T) {
	s := newTestChatStore(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.SetAIWritesEnabled(u.ID, true)
	_ = s.UpsertAIPolicy(u.ID, 1, "db", "t", store.AIPolicy{Insert: true})

	db := setupTestSQLiteWithT(t)
	g := &fakeGateway{decision: Decision{Accept: false}}

	out, _ := handleProposeWrite(context.Background(), ExecCtx{
		Store: s, Gateway: g, DB: db, UserID: u.ID, ConnID: 1, DefaultDB: "db",
	}, map[string]any{
		"database":  "db",
		"table":     "t",
		"operation": "INSERT",
		"sql":       "INSERT INTO t VALUES (42)",
	})
	if !strings.Contains(out, "cancelled") {
		t.Fatalf("got %s", out)
	}
}
