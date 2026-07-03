package cmdexec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/amterp/tai/internal/pathutil"
)

// SwapInto replaces dest with staging as atomically as the filesystem allows.
// An existing dest is first renamed aside rather than removed, so a failure
// mid-swap restores the previous state instead of destroying it - callers
// rely on "fetch failed, previous content intact". os.Rename is atomic on
// POSIX within a single filesystem; on EXDEV we fall back to recursive
// copy + remove (a wider failure window, which is why fetches should stage
// on the same filesystem as dest when possible).
func SwapInto(staging, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("creating dest parent: %w", err)
	}

	old := fmt.Sprintf("%s.old-%d", dest, os.Getpid())
	haveOld := false
	if _, err := os.Lstat(dest); err == nil {
		// A leftover .old dir from a crashed prior run would make the rename
		// fail on some platforms; clear it.
		if err := os.RemoveAll(old); err != nil {
			return fmt.Errorf("clearing stale backup %q: %w", old, err)
		}
		if err := os.Rename(dest, old); err != nil {
			return fmt.Errorf("moving existing dest aside: %w", err)
		}
		haveOld = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat dest: %w", err)
	}

	restoreOld := func() {
		if haveOld {
			_ = os.RemoveAll(dest)
			_ = os.Rename(old, dest)
		}
	}

	if err := os.Rename(staging, dest); err == nil {
		if haveOld {
			if err := os.RemoveAll(old); err != nil {
				return fmt.Errorf("removing previous dest content %q: %w", old, err)
			}
		}
		return nil
	} else if !isCrossDeviceErr(err) {
		restoreOld()
		return fmt.Errorf("rename: %w", err)
	}

	// Cross-filesystem fallback: copy then remove. A partial CopyDir failure
	// can leave a half-written dest; restoreOld's RemoveAll(dest) clears it
	// before the old content is renamed back.
	if err := pathutil.CopyDir(staging, dest); err != nil {
		restoreOld()
		return fmt.Errorf("cross-fs copy: %w", err)
	}
	if haveOld {
		if err := os.RemoveAll(old); err != nil {
			return fmt.Errorf("removing previous dest content %q: %w", old, err)
		}
	}
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("removing staging after copy: %w", err)
	}
	return nil
}

func isCrossDeviceErr(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		if errno, ok := linkErr.Err.(syscall.Errno); ok {
			return errno == syscall.EXDEV
		}
	}
	return false
}
