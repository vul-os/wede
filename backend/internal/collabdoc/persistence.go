package collabdoc

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/reearth/ygo/crdt"
)

// DiskPersistence is a ygo provider PersistenceAdapter that seeds each document
// from a file on disk. The provider "room" name is the room-relative file path.
//
// LoadDoc reads the file and returns it as a v1 update whose YText "content"
// holds the file bytes — so a freshly created doc materializes the file's text.
// StoreUpdate is intentionally a no-op for now: debounced write-back to disk is
// wired in the disk<->doc reconciliation slice. Until then, edits live in the
// provider's in-memory doc while at least one client is connected.
type DiskPersistence struct {
	root string
}

// NewDiskPersistence returns an adapter rooted at a room's workspace directory.
func NewDiskPersistence(root string) *DiskPersistence {
	return &DiskPersistence{root: root}
}

// safeJoin resolves rel under root, rejecting any path that escapes root.
func safeJoin(root, rel string) (string, bool) {
	if rel == "" || rel == "." {
		return "", false
	}
	full := filepath.Join(root, rel)
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}

// LoadDoc returns the stored state for a document (room == relative file path),
// or (nil, nil) when there is nothing to seed (new file, missing, traversal,
// or a directory) — the provider then starts an empty doc.
func (p *DiskPersistence) LoadDoc(room string) ([]byte, error) {
	full, ok := safeJoin(p.root, room)
	if !ok {
		return nil, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, nil // missing / unreadable / directory → empty doc
	}

	doc := crdt.New()
	t := doc.GetText(contentField)
	doc.Transact(func(txn *crdt.Transaction) {
		t.Insert(txn, 0, string(data), nil)
	})
	return crdt.EncodeStateAsUpdateV1(doc, nil), nil
}

// StoreUpdate is a no-op until the reconciliation slice wires debounced
// write-back of materialized text to disk.
func (p *DiskPersistence) StoreUpdate(room string, update []byte) error {
	// TODO(wave4-reconcile): debounce + materialize YText "content" to disk via
	// the room's files service; detect external disk changes and re-seed.
	return nil
}
