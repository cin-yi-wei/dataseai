package mysql

import "testing"

func TestRegistry_RegisterAndList(t *testing.T) {
	reg := NewRegistry()
	reg.Register("q1", 42, "SELECT 1", 1, 10)
	entries := reg.List(1)
	if len(entries) != 1 {
		t.Fatalf("got %d", len(entries))
	}
	if entries[0].QueryID != "q1" || entries[0].ConnectionID != 42 || entries[0].ConnID != 10 {
		t.Fatalf("entry = %+v", entries[0])
	}
}

func TestRegistry_UnregisterRemoves(t *testing.T) {
	reg := NewRegistry()
	reg.Register("q1", 42, "SELECT 1", 1, 10)
	reg.Unregister("q1")
	if len(reg.List(1)) != 0 {
		t.Fatal("expected 0 after unregister")
	}
}

func TestRegistry_ScopedToUser(t *testing.T) {
	reg := NewRegistry()
	reg.Register("q1", 42, "alice query", 1, 10)
	reg.Register("q2", 43, "bob query", 2, 11)
	if list := reg.List(1); len(list) != 1 || list[0].QueryID != "q1" {
		t.Fatalf("alice list = %+v", list)
	}
	if list := reg.List(2); len(list) != 1 || list[0].QueryID != "q2" {
		t.Fatalf("bob list = %+v", list)
	}
}

func TestRegistry_Find(t *testing.T) {
	reg := NewRegistry()
	reg.Register("q1", 42, "x", 1, 10)
	e, ok := reg.Find(1, "q1")
	if !ok {
		t.Fatal("not found")
	}
	if e.ConnectionID != 42 {
		t.Fatalf("conn id = %d", e.ConnectionID)
	}
	if _, ok := reg.Find(2, "q1"); ok {
		t.Fatal("cross-user lookup should fail")
	}
}

func TestRegistry_SQLExcerptIsBounded(t *testing.T) {
	reg := NewRegistry()
	long := "select "
	for len(long) <= 220 {
		long += "x"
	}
	reg.Register("q1", 42, long, 1, 10)
	got := reg.List(1)[0].SQLExcerpt
	if len(got) > 203 {
		t.Fatalf("excerpt too long: %d", len(got))
	}
}
