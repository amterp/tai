//go:build windows

package cmdexec

import "os/exec"

// applyProcessGroup is a no-op on Windows. Job-object-based process group
// cleanup would be the equivalent here, but user-command support is
// effectively POSIX-only today (the existing tests all skip on Windows
// because they assume /bin/sh). If Windows support is added later, this is
// where the job-object plumbing belongs.
func applyProcessGroup(cmd *exec.Cmd) {}
