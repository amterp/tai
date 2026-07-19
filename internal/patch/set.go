package patch

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Set is the ordered collection of patches belonging to one source.
//
// Order is path-sort order. It matters when two patches target the same file:
// they apply in this order, so the result is reproducible across machines
// regardless of directory read order.
type Set struct {
	sourceName string
	patches    []*Patch
	byName     map[string]*Patch
}

// NewSet builds a Set from patches already in the desired order. Exported for
// tests and callers assembling a Set without touching disk.
func NewSet(sourceName string, patches []*Patch) *Set {
	s := &Set{sourceName: sourceName, byName: make(map[string]*Patch, len(patches))}
	for _, p := range patches {
		s.patches = append(s.patches, p)
		s.byName[p.Name] = p
	}
	return s
}

// LoadDir loads every patch file under dir, which is bound to sourceName. A
// missing directory yields an empty Set - a source may declare `patches:` before
// any patch exists, and that is not a problem to report.
//
// Subdirectories are walked rather than skipped: a patch silently ignored
// because it sat one level down is exactly the kind of quiet no-op this feature
// exists to prevent. A patch's name is its path within dir, minus extension, so
// nesting cannot produce collisions.
func LoadDir(dir, sourceName string) (*Set, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return NewSet(sourceName, nil), nil
	}

	var relPaths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		relPaths = append(relPaths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading patches directory %s: %w", dir, err)
	}
	sort.Strings(relPaths)

	var patches []*Patch
	seen := make(map[string]string) // name -> rel path it came from
	for _, rel := range relPaths {
		p, err := ParseFile(filepath.Join(dir, filepath.FromSlash(rel)), dir, sourceName)
		if err != nil {
			return nil, err
		}
		// foo.yaml and foo.yml would share a name, and therefore a .blessed path.
		if prev, ok := seen[p.Name]; ok {
			return nil, fmt.Errorf("duplicate patch name %q: defined by both %s and %s", p.Name, prev, rel)
		}
		seen[p.Name] = rel
		patches = append(patches, p)
	}

	return NewSet(sourceName, patches), nil
}

// ParseFile reads and validates a single patch file. baseDir is the patch
// directory the patch's name is relative to.
func ParseFile(path, baseDir, sourceName string) (*Patch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading patch file: %w", err)
	}

	var p Patch
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing patch %s: %w", filepath.Base(path), err)
	}

	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return nil, err
	}
	name := filepath.ToSlash(rel)
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")

	p.Name = name
	p.SourceName = sourceName
	p.PatchPath = path
	p.BlessedPath = filepath.Join(baseDir, filepath.FromSlash(name)+BlessedExt)
	p.File = filepath.ToSlash(strings.TrimSpace(p.File))

	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("invalid patch %s: %w", rel, err)
	}
	return &p, nil
}

// SourceName is the source this set's patches apply to.
func (s *Set) SourceName() string {
	if s == nil {
		return ""
	}
	return s.sourceName
}

// Get returns the patch with the given name.
func (s *Set) Get(name string) (*Patch, bool) {
	if s == nil {
		return nil, false
	}
	p, ok := s.byName[name]
	return p, ok
}

// All returns every patch in application order.
func (s *Set) All() []*Patch {
	if s == nil {
		return nil
	}
	return s.patches
}

// Len reports how many patches the set holds.
func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.patches)
}

// Files returns the source-relative paths this set targets, in application
// order and deduplicated. Used to drive view materialization.
func (s *Set) Files() []string {
	if s == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, p := range s.patches {
		if !seen[p.File] {
			seen[p.File] = true
			out = append(out, p.File)
		}
	}
	return out
}

// ForFile returns the patches targeting exactly rel, in application order.
func (s *Set) ForFile(rel string) []*Patch {
	if s == nil {
		return nil
	}
	rel = filepath.ToSlash(rel)
	var out []*Patch
	for _, p := range s.patches {
		if p.File == rel {
			out = append(out, p)
		}
	}
	return out
}
