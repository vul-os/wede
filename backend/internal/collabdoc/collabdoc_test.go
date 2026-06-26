package collabdoc

import (
	"testing"

	"github.com/reearth/ygo/crdt"
)

// TestYgoSmoke proves the ygo dependency works in our environment and pins the
// API shape the DocStore will build on: create a Doc, get a named text type,
// edit it inside a transaction, and read the materialized string.
func TestYgoSmoke(t *testing.T) {
	doc := newDoc()
	txt := doc.GetText("content")

	doc.Transact(func(txn *crdt.Transaction) {
		txt.Insert(txn, 0, "World", nil)
		txt.Insert(txn, 0, "Hello ", nil)
	})

	if got := txt.ToString(); got != "Hello World" {
		t.Fatalf("ToString() = %q, want %q", got, "Hello World")
	}
}

// TestYgoUpdateRoundTrip proves two independent docs converge by exchanging an
// encoded update — the foundation of server<->client sync.
func TestYgoUpdateRoundTrip(t *testing.T) {
	a := newDoc()
	at := a.GetText("content")
	a.Transact(func(txn *crdt.Transaction) {
		at.Insert(txn, 0, "shared text", nil)
	})

	// Encode A's full state as a v1 update and apply it to a fresh doc B.
	update := crdt.EncodeStateAsUpdateV1(a, nil)

	b := newDoc()
	if err := crdt.ApplyUpdateV1(b, update, nil); err != nil {
		t.Fatalf("ApplyUpdateV1: %v", err)
	}

	if got := b.GetText("content").ToString(); got != "shared text" {
		t.Fatalf("after sync B = %q, want %q", got, "shared text")
	}
}
