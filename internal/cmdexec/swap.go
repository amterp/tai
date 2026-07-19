package cmdexec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/amterp/tai/internal/pathutil"
)

// staleBackupAge is how long a .old backup must have gone untouched before the
// sweep treats it as abandoned. Comfortably longer than any real swap, and
// short enough that debris does not survive a working session.
const staleBackupAge = 30 * time.Minute

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

	// Sweep backup dirs left behind by crashed prior runs - nothing else ever
	// targets an orphaned .old dir again, so without this they accumulate
	// forever.
	//
	// Only ones old enough to be certain they are abandoned. A concurrent tai
	// swapping the same dest has a backup that is seconds old and is its only
	// way back if it fails; deleting that can leave dest missing entirely. An
	// age check costs nothing here and needs no process introspection, so it
	// works the same on every platform.
	if stale, err := filepath.Glob(dest + ".old-*"); err == nil {
		for _, s := range stale {
			info, statErr := os.Stat(s)
			if statErr != nil || time.Since(info.ModTime()) < staleBackupAge {
				continue
			}
			_ = os.RemoveAll(s)
		}
	}

	old := fmt.Sprintf("%s.old-%d", dest, os.Getpid())
	haveOld := false
	if _, err := os.Lstat(dest); err == nil {
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
	//
	// Known divergence: rename preserves symlinks in staging, CopyDir skips
	// them. Harmless in practice - the install-time copy paths reject
	// symlinks anyway, so they never reach a target either way - but scripts
	// should not rely on symlinks surviving a fetch.
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
