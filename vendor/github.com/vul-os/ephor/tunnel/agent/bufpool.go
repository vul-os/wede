package agent

import (
	"io"
	"sync"
)

// bufpool.go — EFFICIENCY: the agent forwards ALL of a box's public traffic to its
// local app, so its copy paths (WebSocket splice, raw duplex) must not allocate per
// transfer. io.Copy allocates a fresh 32 KiB buffer each call when neither side has
// a ReaderFrom/WriterTo fast-path (a yamux stream has none); we reuse a pool of
// 64 KiB buffers via io.CopyBuffer instead, so steady-state proxying does zero
// per-request buffer allocation.
const copyBufSize = 64 << 10 // 64 KiB

var copyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, copyBufSize)
		return &b
	},
}

// pooledCopy is io.Copy with a pooled scratch buffer (no per-call allocation),
// still honoring any ReaderFrom/WriterTo fast-path io.CopyBuffer can use.
func pooledCopy(dst io.Writer, src io.Reader) (int64, error) {
	bp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bp)
	return io.CopyBuffer(dst, src, *bp)
}
