package install

import (
	"fmt"
	"strings"
)

// ConflictDetail describes a single file conflict detected during installation.
type ConflictDetail struct {
	Path            string // Target-relative path (e.g., "output-styles/test.md")
	FullPath        string // Absolute path on disk
	Reason          string // Human-readable reason (e.g., "unmanaged file exists")
	ResolvedContent []byte // What tai would install (nil for directories)
	ExistingContent []byte // What's currently on disk (nil for directories)
	IsDir           bool
}

// ConflictsError is returned when one or more file conflicts prevent installation.
// It carries structured data so callers can inspect individual conflicts and
// optionally generate diffs.
type ConflictsError struct {
	Conflicts []ConflictDetail
}

func (e *ConflictsError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d conflict(s) detected:\n", len(e.Conflicts))
	for _, c := range e.Conflicts {
		fmt.Fprintf(&b, "  - %s: %s\n", c.Path, c.Reason)
	}
	return strings.TrimSuffix(b.String(), "\n")
}
