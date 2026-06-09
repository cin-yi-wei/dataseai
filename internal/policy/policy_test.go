package policy

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/conray/dataseai/internal/db"
	"github.com/conray/dataseai/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	sdb, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { sdb.Close() })
	if err := store.Migrate(sdb); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &store.Store{DB: sdb}
}

func TestCheckMasterDisabled(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.UpsertAIPolicy(u.ID, 1, "db", "t", store.AIPolicy{Insert: true})

	d := Check(s, u.ID, 1, "db", "t", db.OpInsert, store.ScopeAI)
	if d.Allowed {
		t.Fatal("master off should deny")
	}
	if d.Reason != "master_disabled" {
		t.Fatalf("reason=%q", d.Reason)
	}
}

func TestCheckMissingPolicy(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.SetAIWritesEnabled(u.ID, true)
	d := Check(s, u.ID, 1, "db", "missing", db.OpInsert, store.ScopeAI)
	if d.Allowed {
		t.Fatal("missing row should deny")
	}
	if d.Reason != "policy_denied" {
		t.Fatalf("reason=%q", d.Reason)
	}
}

func TestCheckPerOp(t *testing.T) {
	s := newStore(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.SetAIWritesEnabled(u.ID, true)
	_ = s.UpsertAIPolicy(u.ID, 1, "db", "t", store.AIPolicy{Insert: true, Delete: true, DDL: true})

	cases := []struct {
		op    db.Op
		allow bool
	}{
		{db.OpInsert, true},
		{db.OpUpdate, false},
		{db.OpDelete, true},
		{db.OpTruncate, true},
		{db.OpDDL, true},
		{db.OpSelect, false},
		{db.OpForbidden, false},
		{db.OpUnknown, false},
	}
	for _, c := range cases {
		d := Check(s, u.ID, 1, "db", "t", c.op, store.ScopeAI)
		if d.Allowed != c.allow {
			t.Errorf("%v: allowed=%v want %v", c.op, d.Allowed, c.allow)
		}
	}
}
