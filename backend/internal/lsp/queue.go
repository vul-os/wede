package lsp

import (
	"errors"
	"sync"
	"time"
)

const (
	maxMessageBytes = 8 << 20
	maxHeaderBytes  = 8 << 10
	maxQueuedBytes  = 16 << 20
	// Charge metadata too: zero-length/tiny messages must not create an
	// unbounded number of slice/list allocations within the byte budget.
	queueMessageOverhead = 64
	queueGrace           = 2 * time.Second
)

var errQueueOverloaded = errors.New("LSP overloaded: stdin queue remained full")

// messageQueue accounts for queued AND currently-writing messages. The socket
// reader may additionally hold one bounded message while waiting for space.
// Waits are bounded and cancellable; no lock is held across pipe/network I/O.
type messageQueue struct {
	mu       sync.Mutex
	messages [][]byte
	bytes    int
	changed  chan struct{}
}

func newMessageQueue() *messageQueue { return &messageQueue{changed: make(chan struct{})} }
func (q *messageQueue) notify()      { close(q.changed); q.changed = make(chan struct{}) }

func (q *messageQueue) push(msg []byte, stop <-chan struct{}, grace time.Duration) error {
	cost := len(msg) + queueMessageOverhead
	if len(msg) > maxMessageBytes {
		return errQueueOverloaded
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return errors.New("LSP session stopped")
		default:
		}
		q.mu.Lock()
		if q.bytes+cost <= maxQueuedBytes {
			q.messages = append(q.messages, msg)
			q.bytes += cost
			q.notify()
			q.mu.Unlock()
			return nil
		}
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-stop:
			return errors.New("LSP session stopped")
		case <-timer.C:
			return errQueueOverloaded
		case <-changed:
		}
	}
}

func (q *messageQueue) pop(stop <-chan struct{}) ([]byte, bool) {
	for {
		select {
		case <-stop:
			return nil, false
		default:
		}
		q.mu.Lock()
		if len(q.messages) != 0 {
			msg := q.messages[0]
			q.messages[0] = nil
			q.messages = q.messages[1:]
			q.mu.Unlock()
			return msg, true
		}
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-stop:
			return nil, false
		case <-changed:
		}
	}
}

func (q *messageQueue) written(msg []byte) {
	q.mu.Lock()
	q.bytes -= len(msg) + queueMessageOverhead
	q.notify()
	q.mu.Unlock()
}
