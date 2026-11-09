//go:build !windows

package processguard

import (
	"os/exec"
	"syscall"
)

func prepare(cmd *exec.Cmd) error           { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}; return nil }
func started(cmd *exec.Cmd) (func(), error) { return func() { terminate(cmd) }, nil }
func terminate(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
