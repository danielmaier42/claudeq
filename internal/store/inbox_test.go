package store

import (
	"fmt"
	"testing"
	"time"
)

func TestInboxKeepsOrderAndReadFlag(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := s.Inbox()
	if err != nil || len(got) != 0 {
		t.Fatalf("empty inbox = %v, %v", got, err)
	}
	now := time.Now()
	for i := 1; i <= 3; i++ {
		e := InboxEntry{ID: fmt.Sprintf("i-%d", i), Kind: InboxKindFailure, Title: "t", Message: "m", SentAt: now.Add(time.Duration(i) * time.Second)}
		if err := s.AddInboxEntry(e); err != nil {
			t.Fatalf("AddInboxEntry %d: %v", i, err)
		}
	}
	if err := s.UpdateInbox(func(list *[]InboxEntry) error {
		(*list)[1].Read = true
		return nil
	}); err != nil {
		t.Fatalf("UpdateInbox: %v", err)
	}
	got, err = s.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(got) != 3 || got[0].ID != "i-1" || got[2].ID != "i-3" {
		t.Fatalf("inbox order = %+v", got)
	}
	if got[0].Read || !got[1].Read || got[2].Read {
		t.Fatalf("read flags = %v %v %v, want only the second read", got[0].Read, got[1].Read, got[2].Read)
	}
}

func TestInboxRefusesDuplicateAndMissingIDs(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.AddInboxEntry(InboxEntry{Title: "no id"}); err == nil {
		t.Fatal("an entry without an id was accepted")
	}
	if err := s.AddInboxEntry(InboxEntry{ID: "i-1"}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := s.AddInboxEntry(InboxEntry{ID: "i-1"}); err == nil {
		t.Fatal("a duplicate id was accepted")
	}
}

func TestInboxDropsTheOldestBeyondTheLimit(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for i := 0; i < InboxLimit+5; i++ {
		if err := s.AddInboxEntry(InboxEntry{ID: fmt.Sprintf("i-%03d", i)}); err != nil {
			t.Fatalf("AddInboxEntry %d: %v", i, err)
		}
	}
	got, err := s.Inbox()
	if err != nil {
		t.Fatalf("Inbox: %v", err)
	}
	if len(got) != InboxLimit {
		t.Fatalf("inbox holds %d entries, want %d", len(got), InboxLimit)
	}
	if got[0].ID != "i-005" || got[len(got)-1].ID != fmt.Sprintf("i-%03d", InboxLimit+4) {
		t.Fatalf("the oldest entries were not the ones dropped: first %q, last %q", got[0].ID, got[len(got)-1].ID)
	}
}
