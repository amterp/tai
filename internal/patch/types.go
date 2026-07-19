// Package patch applies user-authored search/replace edits to files coming from
// sources the user does not control.
//
// A patch set belongs to a single source, declared via that source's `patches:`
// field. Patches are applied when tai materializes a patched *view* of the
// source; the pristine source is never written to, so it stays an honest mirror
// of upstream and drift detection has something trustworthy to compare against.
//
// Every edit must match its target exactly once. Matching zero times or more
// than once is an error rather than a best-effort application: a patch that
// silently stops applying, or applies somewhere unintended, is worse than an
// install that stops and says so.
package patch

import (
	"fmt"
	"os"
	"strings"
)

// BlessedExt is the extension of the snapshot recording the upstream content a
// patch was last reviewed against.
const BlessedExt = ".blessed"

// Edit is a single search/replace operation. An empty Replace deletes Find.
type Edit struct {
	Find    string `yaml:"find"`
	Replace string `yaml:"replace"`
}

// Patch is one patch file: a set of edits targeting a single file in a source.
type Patch struct {
	Version     int    `yaml:"version"`
	File        string `yaml:"file"` // source-relative path, e.g. "skills/rad/SKILL.md"
	Description string `yaml:"description,omitempty"`
	Edits       []Edit `yaml:"edits"`

	// Derived at load time, not part of the file format.
	Name        string // path within the patch dir, minus extension
	SourceName  string // source this patch's dir is bound to
	PatchPath   string // absolute path to the .yaml
	BlessedPath string // absolute path to the .blessed snapshot (may not exist)
}

// LoadBlessed reads the blessed snapshot. The bool reports whether one exists;
// a missing snapshot is not an error, it just means the patch is unblessed.
func (p *Patch) LoadBlessed() ([]byte, bool, error) {
	data, err := os.ReadFile(p.BlessedPath)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading blessed snapshot for patch %q: %w", p.Name, err)
	}
	return data, true, nil
}

// FileRef renders the patch's target for display, qualified by its source.
func (p *Patch) FileRef() string {
	return p.SourceName + ":" + p.File
}

func (p *Patch) validate() error {
	if p.Version != 1 {
		return fmt.Errorf("unsupported version %d (expected 1)", p.Version)
	}
	if strings.TrimSpace(p.File) == "" {
		return fmt.Errorf("'file' is required")
	}
	if err := validateTargetPath(p.File); err != nil {
		return err
	}
	if len(p.Edits) == 0 {
		return fmt.Errorf("at least one edit is required")
	}
	for i, e := range p.Edits {
		if e.Find == "" {
			return fmt.Errorf("edit #%d: 'find' cannot be empty", i+1)
		}
	}
	return nil
}

// validateTargetPath mirrors config.validateDestPath: a patch target must stay
// inside its source.
func validateTargetPath(rel string) error {
	if strings.HasPrefix(rel, "/") {
		return fmt.Errorf("'file' path cannot be absolute: %q", rel)
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == ".." {
			return fmt.Errorf("'file' path cannot contain path traversal (..): %q", rel)
		}
	}
	return nil
}
