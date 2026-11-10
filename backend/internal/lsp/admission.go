package lsp

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxSessions = 4
	admissionWait      = 750 * time.Millisecond
)

// Set WEDE_LSP_MAX_SESSIONS to a positive integer before starting the backend.
// All handlers share the same counter, including processes still tearing down.
// Read at admission rather than package init so embedded callers can configure
// the environment before serving. Runtime reconfiguration is not supported.
func maxSessions() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("WEDE_LSP_MAX_SESSIONS")))
	if err != nil || n < 1 {
		return defaultMaxSessions
	}
	return n
}

type sessionLimiter struct {
	mu      sync.Mutex
	active  int
	changed chan struct{}
}

var globalSessions = sessionLimiter{changed: make(chan struct{})}

func (l *sessionLimiter) acquire(cancel <-chan struct{}, limit int, wait time.Duration) (func(), error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		select {
		case <-cancel:
			return nil, fmt.Errorf("LSP session closed or workspace changed")
		default:
		}
		l.mu.Lock()
		if l.active < limit {
			l.active++
			l.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					l.mu.Lock()
					l.active--
					close(l.changed)
					l.changed = make(chan struct{})
					l.mu.Unlock()
				})
			}, nil
		}
		changed := l.changed
		l.mu.Unlock()
		select {
		case <-cancel:
			return nil, fmt.Errorf("LSP session closed or workspace changed")
		case <-timer.C:
			return nil, fmt.Errorf("LSP overloaded: global session limit (%d) reached; retry shortly", limit)
		case <-changed:
		}
	}
}
