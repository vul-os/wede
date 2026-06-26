package collabdoc

import (
	"fmt"
	"sync"

	"github.com/reearth/ygo/crdt"
)

// contentField is the YText name holding a file's text inside its CRDT doc. The
// frontend's y-codemirror binding must bind to this same name.
const contentField = "content"

// DocStore holds the server-authoritative CRDT document for each open file in a
// room, keyed by the file's room-relative path. It is the single source of truth
// for live collaborative content; the disk file is materialized from it
// (debounced) in a later slice, and remote peers sync against it.
//
// All document operations are serialized under one mutex. For a small team
// sharing one host this is simple and correct; per-doc locking can come later if
// it ever matters.
type DocStore struct {
	mu   sync.Mutex
	docs map[string]*crdt.Doc
}

// NewDocStore returns an empty store.
func NewDocStore() *DocStore {
	return &DocStore{docs: make(map[string]*crdt.Doc)}
}

// Open returns the live doc for path, creating and seeding it from content on
// first access. Later calls return the existing doc and ignore content (the doc
// is now authoritative; reseeding would clobber unsynced edits).
func (s *DocStore) Open(path string, content []byte) *crdt.Doc {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.docs[path]; ok {
		return d
	}
	d := crdt.New()
	if len(content) > 0 {
		t := d.GetText(contentField)
		d.Transact(func(txn *crdt.Transaction) {
			t.Insert(txn, 0, string(content), nil)
		})
	}
	s.docs[path] = d
	return d
}

// Text returns the materialized content of an open doc (false if not open).
func (s *DocStore) Text(path string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[path]
	if !ok {
		return "", false
	}
	return d.GetText(contentField).ToString(), true
}

// Encode returns the doc's full state as a v1 update — the payload a newly
// connected peer applies to converge.
func (s *DocStore) Encode(path string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[path]
	if !ok {
		return nil, false
	}
	return crdt.EncodeStateAsUpdateV1(d, nil), true
}

// ApplyUpdate merges a remote v1 update into the open doc.
func (s *DocStore) ApplyUpdate(path string, update []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[path]
	if !ok {
		return fmt.Errorf("collabdoc: doc not open: %s", path)
	}
	return crdt.ApplyUpdateV1(d, update, nil)
}

// IsOpen reports whether a doc is currently open for path.
func (s *DocStore) IsOpen(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.docs[path]
	return ok
}

// Close drops a single doc.
func (s *DocStore) Close(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, path)
}

// CloseAll drops every doc (called on room shutdown).
func (s *DocStore) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = make(map[string]*crdt.Doc)
}

// OpenCount returns the number of open docs.
func (s *DocStore) OpenCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.docs)
}
