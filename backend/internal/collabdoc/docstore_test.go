package collabdoc

import "testing"

func TestDocStoreSeedAndText(t *testing.T) {
	s := NewDocStore()
	s.Open("a.txt", []byte("hello world"))

	if got, ok := s.Text("a.txt"); !ok || got != "hello world" {
		t.Fatalf("Text = %q (ok=%v), want %q", got, ok, "hello world")
	}
	if s.OpenCount() != 1 {
		t.Fatalf("OpenCount = %d, want 1", s.OpenCount())
	}

	// Re-opening must NOT reseed (the live doc is authoritative).
	s.Open("a.txt", []byte("DIFFERENT"))
	if got, _ := s.Text("a.txt"); got != "hello world" {
		t.Errorf("re-Open clobbered content: %q", got)
	}
	if s.OpenCount() != 1 {
		t.Errorf("re-Open created a duplicate: count=%d", s.OpenCount())
	}
}

func TestDocStoreTextMissing(t *testing.T) {
	s := NewDocStore()
	if _, ok := s.Text("nope.txt"); ok {
		t.Error("Text on unopened path should return ok=false")
	}
	if s.IsOpen("nope.txt") {
		t.Error("IsOpen should be false for unopened path")
	}
}

func TestDocStoreEncodeApplyConverges(t *testing.T) {
	// One store seeds a doc; another applies its encoded state and must converge.
	src := NewDocStore()
	src.Open("a.txt", []byte("shared content"))
	update, ok := src.Encode("a.txt")
	if !ok || len(update) == 0 {
		t.Fatalf("Encode failed (ok=%v len=%d)", ok, len(update))
	}

	dst := NewDocStore()
	dst.Open("a.txt", nil) // empty peer
	if err := dst.ApplyUpdate("a.txt", update); err != nil {
		t.Fatalf("ApplyUpdate: %v", err)
	}
	if got, _ := dst.Text("a.txt"); got != "shared content" {
		t.Fatalf("after sync dst = %q, want %q", got, "shared content")
	}
}

func TestDocStoreApplyUpdateUnopened(t *testing.T) {
	s := NewDocStore()
	if err := s.ApplyUpdate("missing.txt", []byte{0}); err == nil {
		t.Error("ApplyUpdate on unopened doc should error")
	}
}

func TestDocStoreCloseAndCloseAll(t *testing.T) {
	s := NewDocStore()
	s.Open("a.txt", []byte("a"))
	s.Open("b.txt", []byte("b"))
	if s.OpenCount() != 2 {
		t.Fatalf("OpenCount = %d, want 2", s.OpenCount())
	}

	s.Close("a.txt")
	if s.IsOpen("a.txt") || s.OpenCount() != 1 {
		t.Fatalf("after Close: open=%v count=%d", s.IsOpen("a.txt"), s.OpenCount())
	}

	s.CloseAll()
	if s.OpenCount() != 0 {
		t.Errorf("after CloseAll OpenCount = %d, want 0", s.OpenCount())
	}
}

func TestDocStoreEmptySeed(t *testing.T) {
	s := NewDocStore()
	s.Open("empty.txt", nil)
	if got, ok := s.Text("empty.txt"); !ok || got != "" {
		t.Errorf("empty seed Text = %q (ok=%v), want empty string", got, ok)
	}
}
