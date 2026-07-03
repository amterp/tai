//go:build unix

package cmdexec

import (
	"os/exec"
	"syscall"
	"time"
)

// applyProcessGroup puts the command in its own process group so the kernel
// can be told to signal the whole group on cancellation. Without this, a
// timeout cancellation only SIGKILLs the direct child - a manifest like
// command: ["sh", "-c", "curl ..."] would leave curl running as an orphan.
//
// On Go 1.20+, exec.Cmd.Cancel runs when the context is cancelled. We use it
// to send SIGKILL to the negative PID (process group). WaitDelay gives the
// group a brief grace period before exec.Cmd.Run returns, after which any
// stragglers blocking on stdio pipes are forcibly disconnected.
func applyProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}
