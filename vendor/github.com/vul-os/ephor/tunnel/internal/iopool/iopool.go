// Package iopool holds the single pooled-buffer copy helper shared by the relay
// server and the box agent.
//
// EFFICIENCY: both ends of the tunnel forward ALL box/app traffic, so neither
// per-byte forwarding path may allocate. io.Copy allocates a fresh 32 KiB scratch
// buffer on every call when the source/destination expose no ReaderFrom/WriterTo
// fast-path (a yamux stream exposes neither) — on a busy relay that is one 32 KiB
// allocation PER request body and PER WebSocket direction, and bytes are direct
// COGS. Copy reuses a pool of fixed buffers via io.CopyBuffer instead, so
// steady-state forwarding does zero per-request buffer allocation.
//
// The buffer size (64 KiB) matches the agent's bufio reader and is a good tradeoff
// between syscall count and memory: larger buffers cut read/write syscalls on big
// transfers without bloating per-stream memory. Buffers are returned to the pool
// after each copy, so concurrent streams reuse a small working set rather than each
// holding its own.
//
// DEDUP: `tunnel/server/bufpool.go` and `tunnel/agent/bufpool.go` were two copies of
// this file — same constant, same pool, same function, differing only in the package
// clause and the wording of the comments. They are one implementation here now;
// server and agent still get independent pools' worth of reuse from a single
// sync.Pool, which is per-P sharded internally and is not a contention point.
package iopool

import (
	"io"
	"sync"
)

// BufSize is the pooled scratch-buffer size. See the package doc for why 64 KiB.
const BufSize = 64 << 10 // 64 KiB

var pool = sync.Pool{
	New: func() any {
		b := make([]byte, BufSize)
		return &b
	},
}

// Copy is io.Copy with a pooled scratch buffer — no per-call allocation. It still
// honors any ReaderFrom/WriterTo fast-path io.CopyBuffer detects (e.g. a
// splice-capable *net.TCPConn), falling back to the pooled buffer otherwise.
func Copy(dst io.Writer, src io.Reader) (int64, error) {
	bp := pool.Get().(*[]byte)
	defer pool.Put(bp)
	return io.CopyBuffer(dst, src, *bp)
}
