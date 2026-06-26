package presence

import (
	"encoding/json"
	"testing"
)

// lastRoster drains a channel non-blockingly and returns the members from the
// most recent roster event (nil if none pending).
func lastRoster(t *testing.T, ch <-chan []byte) []Member {
	t.Helper()
	var members []Member
	got := false
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return members
			}
			var ev rosterEvent
			if err := json.Unmarshal(data, &ev); err != nil {
				t.Fatalf("bad roster json: %v", err)
			}
			members = ev.Members
			got = true
		default:
			if !got {
				return nil
			}
			return members
		}
	}
}

func find(members []Member, id string) *Member {
	for i := range members {
		if members[i].ID == id {
			return &members[i]
		}
	}
	return nil
}

func TestJoinAssignsDistinctIDAndColor(t *testing.T) {
	h := NewHub()
	id1, ch1 := h.Join("alice")
	id2, _ := h.Join("bob")

	if id1 == id2 {
		t.Fatal("expected distinct ids")
	}
	roster := h.Roster()
	if len(roster) != 2 {
		t.Fatalf("roster len = %d, want 2", len(roster))
	}
	a, b := find(roster, id1), find(roster, id2)
	if a == nil || b == nil {
		t.Fatal("members missing from roster")
	}
	if a.Color == b.Color {
		t.Errorf("expected distinct colors, both %s", a.Color)
	}
	if a.Username != "alice" {
		t.Errorf("username = %q, want alice", a.Username)
	}

	// alice's channel should have received roster broadcasts (her join, then bob's).
	got := lastRoster(t, ch1)
	if len(got) != 2 {
		t.Fatalf("alice last roster len = %d, want 2", len(got))
	}
}

func TestUpdateReflectsInRoster(t *testing.T) {
	h := NewHub()
	id, ch := h.Join("alice")

	h.Update(id, "src/main.go", 42)

	got := lastRoster(t, ch)
	m := find(got, id)
	if m == nil {
		t.Fatal("member missing")
	}
	if m.File != "src/main.go" || m.Line != 42 {
		t.Errorf("update not reflected: file=%q line=%d", m.File, m.Line)
	}
}

func TestLeaveRemovesAndClosesChannel(t *testing.T) {
	h := NewHub()
	id1, ch1 := h.Join("alice")
	id2, _ := h.Join("bob")

	h.Leave(id2)
	if len(h.Roster()) != 1 {
		t.Fatalf("roster len = %d, want 1 after leave", len(h.Roster()))
	}

	// alice still present; her channel still open and shows only her.
	got := lastRoster(t, ch1)
	if len(got) != 1 || find(got, id1) == nil {
		t.Fatalf("alice roster wrong after bob left: %+v", got)
	}

	// Leaving id2 again is a no-op (does not panic).
	h.Leave(id2)
}

func TestCloseClosesAllChannels(t *testing.T) {
	h := NewHub()
	_, ch1 := h.Join("alice")
	_, ch2 := h.Join("bob")

	h.Close()

	// Drain; both channels must be closed.
	drain := func(ch <-chan []byte) bool {
		for {
			_, ok := <-ch
			if !ok {
				return true
			}
		}
	}
	if !drain(ch1) || !drain(ch2) {
		t.Fatal("channels not closed after Close")
	}
	if len(h.Roster()) != 0 {
		t.Errorf("roster not empty after Close: %d", len(h.Roster()))
	}

	// Join after Close returns a closed channel and empty id.
	id, ch := h.Join("late")
	if id != "" {
		t.Errorf("expected empty id after Close, got %q", id)
	}
	if _, ok := <-ch; ok {
		t.Error("expected closed channel from Join after Close")
	}
}
