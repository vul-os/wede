//go:build linux || darwin

package lsp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The descendant keeps both stdout and a TCP connection open forever. EOF on
// the TCP connection proves it stopped running, without mistaking an orphaned
// zombie awaiting init's reap for a live process (common in containers).
func TestLSPDescendant(t *testing.T) {
	address := os.Getenv("WEDE_LSP_DESCENDANT")
	if address == "" {
		return
	}
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		os.Exit(4)
	}
	fmt.Fprintln(conn, os.Getpid())
	for {
		time.Sleep(time.Hour)
	}
}

func TestDescendantCleanup(t *testing.T) {
	for _, action := range []string{"disconnect", "parent-exit", "Close", "workspace-change"} {
		t.Run(action, func(t *testing.T) {
			h, ws, url := lifecycleHarness(t)
			c := dialLSP(t, url)
			p := processFor(t, h, request(t, c, 1, "initialize", "fresh").PID)
			// A second group must survive cleanup of the first.
			other := dialLSP(t, url)
			request(t, other, 1, "initialize", "other group")
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
			request(t, c, 2, "spawn", listener.Addr().String())
			child, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer child.Close()
			child.SetReadDeadline(time.Now().Add(5 * time.Second))
			reader := bufio.NewReader(child)
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(line))
			if err != nil {
				t.Fatal(err)
			}
			// Best-effort cleanup on assertion failure, not the mechanism under test.
			terminated := false
			t.Cleanup(func() {
				if terminated {
					return
				}
				process, err := os.FindProcess(pid)
				if err == nil {
					process.Kill()
				}
			})
			switch action {
			case "disconnect":
				c.Close()
			case "parent-exit":
				c.WriteJSON(map[string]any{"method": "exit"})
			case "Close":
				h.Close()
			case "workspace-change":
				ws.change(t.TempDir())
			}
			awaitCleanup(t, h, p)
			if _, err := reader.ReadByte(); err != io.EOF {
				t.Fatalf("descendant still holds connection: %v", err)
			}
			terminated = true
			if action == "disconnect" || action == "parent-exit" {
				request(t, other, 2, "hover", "survived")
			}
		})
	}
}
