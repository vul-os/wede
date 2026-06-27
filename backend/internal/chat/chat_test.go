package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── History persistence ────────────────────────────────────────────────────────

func TestPostAppendsToFile(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(dir, ChannelPublic, "")
	defer h.Close()

	id, ch := h.Join("alice", "#f87171")
	defer h.Leave(id)

	// No prior history → no history frame; drain defensively.
	select {
	case <-ch:
	default:
	}

	h.Post("alice", "#f87171", "hello world")

	select {
	case data := <-ch:
		s := string(data)
		if !strings.Contains(s, "hello world") {
			t.Fatalf("expected message in payload, got: %s", s)
		}
		if !strings.Contains(s, `"kind":"user"`) {
			t.Fatalf("expected kind=user in payload, got: %s", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	chatFile := filepath.Join(dir, ".wede", "chat.md")
	content, err := os.ReadFile(chatFile)
	if err != nil {
		t.Fatalf("chat.md not written: %v", err)
	}
	if !strings.Contains(string(content), "alice: hello world") {
		t.Fatalf("expected 'alice: hello world' in chat.md, got: %s", content)
	}
	if !strings.Contains(string(content), "[user]") {
		t.Fatalf("expected [user] tag in chat.md, got: %s", content)
	}
}

func TestNewHubReplaysHistory(t *testing.T) {
	dir := t.TempDir()

	wedeDir := filepath.Join(dir, ".wede")
	if err := os.MkdirAll(wedeDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := "- 2026-06-26T15:30:00Z [user] alice: hello world\n" +
		"- 2026-06-26T15:31:00Z [git] 📦 committed a1b2c3: fix typo\n"
	if err := os.WriteFile(filepath.Join(wedeDir, "chat.md"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	h := NewHub(dir, ChannelPublic, "")
	defer h.Close()

	if len(h.history) != 2 {
		t.Fatalf("expected 2 history messages, got %d", len(h.history))
	}

	id, ch := h.Join("bob", "#60a5fa")
	defer h.Leave(id)

	select {
	case data := <-ch:
		s := string(data)
		if !strings.Contains(s, `"type":"history"`) {
			t.Fatalf("expected history type, got: %s", s)
		}
		if !strings.Contains(s, "hello world") {
			t.Fatalf("expected history content 'hello world', got: %s", s)
		}
		if !strings.Contains(s, "fix typo") {
			t.Fatalf("expected history content 'fix typo', got: %s", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for history frame")
	}
}

func TestNewHubEmptyHistoryNoFrame(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(dir, ChannelPublic, "")
	defer h.Close()

	_, ch := h.Join("carol", "#fbbf24")

	// No history → no frame should arrive within 100ms.
	select {
	case data := <-ch:
		t.Fatalf("expected no frame for empty history, got: %s", data)
	case <-time.After(100 * time.Millisecond):
		// expected
	}
}

// ── Message kinds ──────────────────────────────────────────────────────────────

func TestPostGitAndSystemKinds(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(dir, ChannelPublic, "")
	defer h.Close()

	id, ch := h.Join("watcher", "#888888")
	defer h.Leave(id)

	select {
	case <-ch:
	default:
	}

	h.PostGit("📦 committed abc1234: initial commit")
	select {
	case data := <-ch:
		s := string(data)
		if !strings.Contains(s, `"kind":"git"`) {
			t.Fatalf("expected kind=git, got: %s", s)
		}
		if !strings.Contains(s, "abc1234") {
			t.Fatalf("expected commit hash in message, got: %s", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for git message")
	}

	h.PostSystem("workspace opened")
	select {
	case data := <-ch:
		s := string(data)
		if !strings.Contains(s, `"kind":"system"`) {
			t.Fatalf("expected kind=system, got: %s", s)
		}
		if !strings.Contains(s, "workspace opened") {
			t.Fatalf("expected system text, got: %s", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for system message")
	}
}

// ── Broadcast to multiple peers ────────────────────────────────────────────────

func TestBroadcastToAllPeers(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(dir, ChannelPublic, "")
	defer h.Close()

	id1, ch1 := h.Join("alice", "#f87171")
	id2, ch2 := h.Join("bob", "#60a5fa")
	defer h.Leave(id1)
	defer h.Leave(id2)

	// Drain any history frames.
	for _, ch := range []<-chan []byte{ch1, ch2} {
		select {
		case <-ch:
		default:
		}
	}

	h.Post("alice", "#f87171", "hi everyone")

	for i, ch := range []<-chan []byte{ch1, ch2} {
		select {
		case data := <-ch:
			if !strings.Contains(string(data), "hi everyone") {
				t.Fatalf("peer %d: expected message, got: %s", i, data)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("peer %d: timed out waiting for message", i)
		}
	}
}

// ── parseLine ──────────────────────────────────────────────────────────────────

func TestParseLineValid(t *testing.T) {
	cases := []struct {
		line string
		kind string
		user string
		text string
	}{
		{
			"- 2026-06-26T15:30:00Z [user] alice: hello world",
			"user", "alice", "hello world",
		},
		{
			"- 2026-06-26T15:31:00Z [git] 📦 committed a1b2c3: fix typo",
			"git", "", "📦 committed a1b2c3: fix typo",
		},
		{
			"- 2026-06-26T15:32:00Z [system] workspace opened",
			"system", "", "workspace opened",
		},
		{
			// message text containing ": " should not be split
			"- 2026-06-26T15:33:00Z [user] dave: key: value",
			"user", "dave", "key: value",
		},
	}
	for _, c := range cases {
		msg, ok := parseLine(c.line)
		if !ok {
			t.Errorf("expected parseLine(%q) to succeed", c.line)
			continue
		}
		if msg.Kind != c.kind {
			t.Errorf("line %q: kind got %q want %q", c.line, msg.Kind, c.kind)
		}
		if msg.User != c.user {
			t.Errorf("line %q: user got %q want %q", c.line, msg.User, c.user)
		}
		if msg.Text != c.text {
			t.Errorf("line %q: text got %q want %q", c.line, msg.Text, c.text)
		}
	}
}

func TestParseLineMalformed(t *testing.T) {
	cases := []string{
		"",
		"not a chat line",
		"- malformed with no kind",
		"- 2026-06-26 [user] alice: missing timezone",
		"- 2026-06-26T15:30:00Z nokind rest",
	}
	for _, c := range cases {
		if _, ok := parseLine(c); ok {
			t.Errorf("expected parseLine(%q) to fail, but it succeeded", c)
		}
	}
}

// ── Git-poll dedup logic ───────────────────────────────────────────────────────

func TestShouldPostCommit(t *testing.T) {
	// First poll: prev is empty string → never post (haven't established a baseline).
	if shouldPostCommit("", "abc123") {
		t.Error("should not post on first poll (no prev HEAD)")
	}
	// Same HEAD as before → no event.
	if shouldPostCommit("abc123", "abc123") {
		t.Error("should not post when HEAD is unchanged")
	}
	// HEAD changed → post.
	if !shouldPostCommit("abc123", "def456") {
		t.Error("should post when HEAD changes")
	}
}

func TestShouldPostDirty(t *testing.T) {
	// First sample (prev=-1) → never post.
	if shouldPostDirty(-1, 3) {
		t.Error("should not post on first sample (prev=-1)")
	}
	// Same count → no event.
	if shouldPostDirty(3, 3) {
		t.Error("should not post when count is unchanged")
	}
	// Count increased → post.
	if !shouldPostDirty(0, 3) {
		t.Error("should post when dirty count increases from 0")
	}
	// Count decreased to 0 → post.
	if !shouldPostDirty(3, 0) {
		t.Error("should post when dirty count drops to 0")
	}
}

// ── Close idempotency ──────────────────────────────────────────────────────────

func TestCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(dir, ChannelPublic, "")
	h.Close()
	h.Close() // second Close must not panic
}

func TestJoinAfterClose(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(dir, ChannelPublic, "")
	h.Close()

	id, ch := h.Join("late", "#888")
	if id != "" {
		t.Errorf("expected empty id after Close, got %q", id)
	}
	// Channel should already be closed.
	select {
	case _, open := <-ch:
		if open {
			t.Error("expected closed channel after joining a closed hub")
		}
	default:
		t.Error("expected channel to be closed (receive should not block)")
	}
}

// TestPrivateChannel verifies the private channel writes to .wede/private/chat.md
// and that wede auto-gitignores the private folder.
func TestPrivateChannel(t *testing.T) {
	dir := t.TempDir()
	pub := NewHub(dir, ChannelPublic, "")
	defer pub.Close()
	priv := NewHub(dir, ChannelPrivate, "")
	defer priv.Close()

	pub.Post("alice", "#fff", "public hello")
	priv.Post("bob", "#000", "private secret")
	time.Sleep(20 * time.Millisecond)

	pubFile := filepath.Join(dir, ".wede", "chat.md")
	privFile := filepath.Join(dir, ".wede", "private", "chat.md")
	if b, _ := os.ReadFile(pubFile); !strings.Contains(string(b), "public hello") {
		t.Errorf("public chat.md missing public message: %q", b)
	}
	if b, _ := os.ReadFile(pubFile); strings.Contains(string(b), "private secret") {
		t.Error("private message leaked into public chat.md")
	}
	if b, _ := os.ReadFile(privFile); !strings.Contains(string(b), "private secret") {
		t.Errorf("private chat.md missing private message: %q", b)
	}
	// .wede/.gitignore should exclude the private folder.
	gi, err := os.ReadFile(filepath.Join(dir, ".wede", ".gitignore"))
	if err != nil || !strings.Contains(string(gi), "private/") {
		t.Errorf(".wede/.gitignore should contain 'private/': err=%v content=%q", err, gi)
	}
}
