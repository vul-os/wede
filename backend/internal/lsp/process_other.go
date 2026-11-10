//go:build !linux && !darwin

package lsp

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}

// Other platforms retain direct-child termination. Process-tree containment
// requires platform-specific facilities (for example, Windows Job Objects).
func killProcessTree(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
