package collabdoc

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/reearth/ygo/crdt"
)

// decodeRoom maps a provider room name back to a room-relative file path. The
// frontend encodes the path as base64url (RawURLEncoding) so it is a single,
// slash-free, URL-safe token for the y-websocket client. Real file paths contain
// characters outside the base64url alphabet ('/', '.'), so a failed decode
// safely falls back to treating the name as a raw path (handy for curl/testing).
func decodeRoom(room string) string {
	if b, err := base64.RawURLEncoding.DecodeString(room); err == nil {
		return string(b)
	}
	if b, err := base64.URLEncoding.DecodeString(room); err == nil {
		return string(b)
	}
	return room
}

// DocProvider is the subset of ygo's provider/websocket Server that the
// persistence layer needs: fetch the live doc for a room. Defined as an interface
// so this package stays decoupled from the provider package.
type DocProvider interface {
	GetDoc(name string) *crdt.Doc
}

// defaultDebounce is how long write-back waits after the last edit before
// flushing materialized text to disk.
const defaultDebounce = 600 * time.Millisecond

// DiskPersistence is a ygo provider PersistenceAdapter backed by files on disk.
// The provider "room" name is a file's room-relative path.
//
//   - LoadDoc seeds a document from the file (so a new doc materializes the
//     file's current text).
//   - StoreUpdate fires on each incremental edit; rather than persist opaque
//     updates, we debounce and write the doc's full materialized text back to the
//     file via the DocProvider (GetDoc -> YText "content" -> ToString). Writes are
//     atomic (temp + rename).
//
// External disk changes (e.g. git checkout, a terminal edit) re-seeding an open
// doc is a separate, later slice — it needs care to avoid feedback loops with the
// write-back below.
type DiskPersistence struct {
	root     string
	debounce time.Duration

	mu       sync.Mutex
	provider DocProvider
	timers   map[string]*time.Timer
	stopped  bool
}

// NewDiskPersistence returns an adapter rooted at a room's workspace directory.
func NewDiskPersistence(root string) *DiskPersistence {
	return &DiskPersistence{
		root:     root,
		debounce: defaultDebounce,
		timers:   make(map[string]*time.Timer),
	}
}

// SetProvider wires the live document source (the ygo Server). Until set,
// StoreUpdate is a no-op.
func (p *DiskPersistence) SetProvider(dp DocProvider) {
	p.mu.Lock()
	p.provider = dp
	p.mu.Unlock()
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
// or (nil, nil) when there is nothing to seed (new/missing file, traversal, or a
// directory) — the provider then starts an empty doc.
func (p *DiskPersistence) LoadDoc(room string) ([]byte, error) {
	full, ok := safeJoin(p.root, decodeRoom(room))
	if !ok {
		return nil, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, nil
	}

	doc := crdt.New()
	t := doc.GetText(contentField)
	doc.Transact(func(txn *crdt.Transaction) {
		t.Insert(txn, 0, string(data), nil)
	})
	return crdt.EncodeStateAsUpdateV1(doc, nil), nil
}

// StoreUpdate schedules a debounced write-back of the room's materialized text.
func (p *DiskPersistence) StoreUpdate(room string, _ []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.provider == nil {
		return nil
	}
	if t, ok := p.timers[room]; ok {
		t.Reset(p.debounce)
		return nil
	}
	p.timers[room] = time.AfterFunc(p.debounce, func() { p.flush(room) })
	return nil
}

// flush materializes the room's current doc text to disk.
func (p *DiskPersistence) flush(room string) {
	p.mu.Lock()
	delete(p.timers, room)
	prov, stopped := p.provider, p.stopped
	p.mu.Unlock()
	if prov == nil || stopped {
		return
	}
	p.materialize(prov, room)
}

// materialize reads the live doc text and writes it to disk atomically.
func (p *DiskPersistence) materialize(prov DocProvider, room string) {
	doc := prov.GetDoc(room)
	if doc == nil {
		return
	}
	text := doc.GetText(contentField).ToString()
	full, ok := safeJoin(p.root, decodeRoom(room))
	if !ok {
		return
	}
	_ = writeAtomic(full, []byte(text))
}

// Stop cancels pending timers and performs a final synchronous flush so the last
// edits are persisted. Called on room shutdown.
func (p *DiskPersistence) Stop() {
	p.mu.Lock()
	p.stopped = true
	rooms := make([]string, 0, len(p.timers))
	for r, t := range p.timers {
		t.Stop()
		rooms = append(rooms, r)
	}
	p.timers = make(map[string]*time.Timer)
	prov := p.provider
	p.mu.Unlock()

	if prov == nil {
		return
	}
	for _, room := range rooms {
		p.materialize(prov, room)
	}
}

// writeAtomic writes data to path via a temp file + rename, creating parent dirs.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wede-collab-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
