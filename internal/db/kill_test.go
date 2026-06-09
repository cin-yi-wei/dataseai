package db

import (
	"errors"
	"testing"
)

func TestKillRegistryRoundTrip(t *testing.T) {
	r := NewKillRegistry()
	r.Register("q1", 123, "SELECT 1", 7, 9)
	q, ok := r.Lookup("q1")
	if !ok {
		t.Fatal("expected to find q1")
	}
	if q.ConnectionID != 123 || q.UserID != 7 || q.ConnID != 9 {
		t.Fatalf("unexpected: %+v", q)
	}
	r.Unregister("q1")
	if _, ok := r.Lookup("q1"); ok {
		t.Fatal("expected q1 cleared after Unregister")
	}
}

func TestKillRegistryAuthorize(t *testing.T) {
	r := NewKillRegistry()
	r.Register("q1", 123, "SELECT 1", 7, 9)
	_, err := r.Authorize("q1", 8) // wrong user
	if !errors.Is(err, ErrKillNoMatch) {
		t.Fatalf("expected ErrKillNoMatch, got %v", err)
	}
	q, err := r.Authorize("q1", 7)
	if err != nil {
		t.Fatal(err)
	}
	if q.ConnectionID != 123 {
		t.Fatalf("connection id = %d", q.ConnectionID)
	}
}
