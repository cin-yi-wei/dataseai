package store

import (
	"testing"
)

func TestAIPolicySetGet(t *testing.T) {
	s := setupUsers(t)
	u, _ := s.CreateUser("alice", "longpassword1")

	p, found, err := s.GetAIPolicy(u.ID, 1, "db1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected !found for missing row")
	}
	if (p != AIPolicy{}) {
		t.Fatalf("expected zero value, got %+v", p)
	}

	if err := s.UpsertAIPolicy(u.ID, 1, "db1", "t1", AIPolicy{Insert: true, Delete: true}); err != nil {
		t.Fatal(err)
	}
	p, found, _ = s.GetAIPolicy(u.ID, 1, "db1", "t1")
	if !found {
		t.Fatal("expected found after upsert")
	}
	if !p.Insert || p.Update || !p.Delete || p.DDL {
		t.Fatalf("unexpected policy: %+v", p)
	}

	if err := s.UpsertAIPolicy(u.ID, 1, "db1", "t1", AIPolicy{Insert: true, Update: true, Delete: true, DDL: true}); err != nil {
		t.Fatal(err)
	}
	p, _, _ = s.GetAIPolicy(u.ID, 1, "db1", "t1")
	if !p.Insert || !p.Update || !p.Delete || !p.DDL {
		t.Fatalf("upsert didn't update: %+v", p)
	}
}

func TestAIPolicyBatchAndList(t *testing.T) {
	s := setupUsers(t)
	u, _ := s.CreateUser("alice", "longpassword1")

	err := s.BatchUpsertAIPolicy(u.ID, 1, "db1", []string{"t1", "t2", "t3"}, AIPolicy{Insert: true})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := s.ListAIPolicy(u.ID, 1, "db1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3, got %d", len(rows))
	}
	for _, r := range rows {
		if !r.Policy.Insert {
			t.Fatalf("%s missing Insert", r.Table)
		}
	}
}

func TestAIPolicyDelete(t *testing.T) {
	s := setupUsers(t)
	u, _ := s.CreateUser("alice", "longpassword1")
	_ = s.UpsertAIPolicy(u.ID, 1, "db1", "t1", AIPolicy{Insert: true})

	if err := s.DeleteAIPolicy(u.ID, 1, "db1", "t1"); err != nil {
		t.Fatal(err)
	}
	_, found, _ := s.GetAIPolicy(u.ID, 1, "db1", "t1")
	if found {
		t.Fatal("expected not found after delete")
	}
}

func TestAIPolicyCrossUserIsolation(t *testing.T) {
	s := setupUsers(t)
	a, _ := s.CreateUser("alice", "longpassword1")
	b, _ := s.CreateUser("bob", "longpassword2")
	_ = s.UpsertAIPolicy(a.ID, 1, "db1", "t1", AIPolicy{Insert: true})

	_, found, _ := s.GetAIPolicy(b.ID, 1, "db1", "t1")
	if found {
		t.Fatal("alice's policy leaked to bob")
	}
}
