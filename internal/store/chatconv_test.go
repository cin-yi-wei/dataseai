package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConversations_CRUDAndScope(t *testing.T) {
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0)

	a, err := s.CreateConversation(1, 10, "dbA", "未命名-1", now)
	if err != nil {
		t.Fatal(err)
	}
	// different db scope
	_, _ = s.CreateConversation(1, 10, "dbB", "other", now)
	// different user
	_, _ = s.CreateConversation(2, 10, "dbA", "someone-else", now)

	list, err := s.ListConversations(1, 10, "dbA")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "未命名-1" {
		t.Fatalf("scope leak: %+v", list)
	}

	// rename
	if err := s.RenameConversation(1, a.ID, "renamed"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameConversation(999, a.ID, "hack"); err != ErrNotFound {
		t.Fatalf("rename by non-owner should fail, got %v", err)
	}

	// messages
	msgs := []StoredChatMessage{
		{Role: "user", Blocks: json.RawMessage(`[{"type":"text","text":"hi"}]`)},
		{Role: "assistant", Blocks: json.RawMessage(`[{"type":"text","text":"hello"}]`)},
	}
	if err := s.ReplaceMessages(1, a.ID, msgs, now); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMessages(1, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Role != "user" || string(got[1].Blocks) != `[{"type":"text","text":"hello"}]` {
		t.Fatalf("messages = %+v", got)
	}

	// non-owner can't read
	if _, err := s.GetMessages(999, a.ID); err != ErrNotFound {
		t.Fatalf("non-owner read should fail, got %v", err)
	}

	// delete
	if err := s.DeleteConversation(1, a.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListConversations(1, 10, "dbA"); len(list) != 0 {
		t.Fatalf("should be empty after delete: %+v", list)
	}
}
