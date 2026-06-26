// Package collabdoc is the server-authoritative CRDT document layer for
// collaborative editing, built on github.com/reearth/ygo (a pure-Go, cgo-free,
// Yjs-v13 wire-compatible CRDT). This file currently just pins the dependency
// and documents the intended shape; the DocStore (one Doc per open file, seeded
// from disk, synced over the collab WebSocket) is built in subsequent slices.
package collabdoc

import "github.com/reearth/ygo/crdt"

// newDoc returns a fresh CRDT document. Kept as a thin wrapper so the rest of
// wede depends on this package rather than ygo's API directly, easing any future
// swap (the Yjs wire format is the real contract).
func newDoc() *crdt.Doc {
	return crdt.New()
}
