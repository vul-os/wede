//go:build linux || darwin

package lsp

import (
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd) {
	// Descendants inherit this group, never the backend's process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessTree(cmd *exec.Cmd) {
	// Signal the group even after Wait reports the leader's exit: descendants
	// may still be running and holding inherited pipes open.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	// Also cover a server that moved itself out of the original group.
	_ = cmd.Process.Kill()
}
