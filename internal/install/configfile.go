package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/amterp/tai/internal/configasset"
)

// configFileSet is a batch of target config files opened for editing.
//
// Config files are shared with the target itself and with unrelated tools, so
// every one is read, mutated in memory, and written back whole - never
// rendered from scratch. Files are also shared between config assets (several
// hooks land in one settings.json), so the set loads each file once and
// flushes it once, rather than letting each asset read-modify-write in turn.
type configFileSet struct {
	docs    map[string]map[string]any
	existed map[string]bool
	dirty   map[string]bool

	// baseline holds each file's content as tai would serialize it at load
	// time, so flush can tell a real change from a no-op. It deliberately
	// records the re-encoded form rather than the bytes on disk: a file whose
	// formatting merely differs from tai's is left alone, because rewriting a
	// file the target may also be writing is a risk taken only for a reason.
	baseline map[string][]byte
}

func newConfigFileSet() *configFileSet {
	return &configFileSet{
		docs:     make(map[string]map[string]any),
		existed:  make(map[string]bool),
		dirty:    make(map[string]bool),
		baseline: make(map[string][]byte),
	}
}

// load returns the parsed document for path, reading it on first use. A
// missing file yields an empty document that flush will create.
func (s *configFileSet) load(path string) (map[string]any, error) {
	if doc, ok := s.docs[path]; ok {
		return doc, nil
	}

	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		s.docs[path] = make(map[string]any)
		s.existed[path] = false
		return s.docs[path], nil
	case err != nil:
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	doc, err := decodeDoc(path, data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	baseline, err := encodeDoc(path, doc)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	s.docs[path] = doc
	s.existed[path] = true
	s.baseline[path] = baseline
	return doc, nil
}

// markDirty records that a file's document was mutated.
func (s *configFileSet) markDirty(path string) { s.dirty[path] = true }

// flush writes back every file whose serialized content actually changed.
// Returns the paths written.
func (s *configFileSet) flush() ([]string, error) {
	var written []string
	for path := range s.dirty {
		doc := s.docs[path]

		encoded, err := encodeDoc(path, doc)
		if err != nil {
			return written, fmt.Errorf("serializing %s: %w", path, err)
		}

		// A no-op edit should not churn the file. This matters most for files
		// tai shares with a running target: every rewrite is a chance to race.
		if s.existed[path] && bytes.Equal(encoded, s.baseline[path]) {
			continue
		}

		if err := writeFileAtomic(path, encoded); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	return written, nil
}

// decodeDoc parses a config file into a generic document, picking the format
// from the file extension. Targets do not agree on one: Codex keeps its config
// in TOML while everyone else uses JSON.
func decodeDoc(path string, data []byte) (map[string]any, error) {
	if isTOML(path) {
		return decodeTOML(data)
	}
	return decodeJSON(data)
}

// encodeDoc renders a config document in the file's own format.
func encodeDoc(path string, doc map[string]any) ([]byte, error) {
	if isTOML(path) {
		return encodeTOML(doc)
	}
	return encodeJSON(doc)
}

// isTOML reports whether a config file is TOML rather than JSON.
func isTOML(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".toml")
}

// decodeTOML parses a TOML config file.
//
// Comments and key ordering do not survive: no Go TOML library round-trips
// them. That loss is why writing a TOML config the user also hand-edits is
// gated behind their explicit consent.
func decodeTOML(data []byte) (map[string]any, error) {
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = make(map[string]any)
	}
	return doc, nil
}

// encodeTOML renders a TOML config file.
func encodeTOML(doc map[string]any) ([]byte, error) {
	return toml.Marshal(doc)
}

// decodeJSON parses a config file into a generic document.
//
// It uses json.Number rather than the default float64 so numbers survive a
// round trip exactly. Without it, reading and rewriting a file like
// ~/.claude.json - which holds large integer timestamps next to the MCP
// servers tai manages - would silently reformat or lose precision on values
// tai never touched.
func decodeJSON(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = make(map[string]any)
	}
	return doc, nil
}

// encodeJSON renders a config document with the two-space indent and trailing
// newline these files conventionally use.
func encodeJSON(doc map[string]any) ([]byte, error) {
	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

// writeFileAtomic writes via a temp file and rename, so a crash or a
// concurrent reader never observes a half-written config file.
func writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tai-*")
	if err != nil {
		return fmt.Errorf("creating temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file for %s: %w", path, err)
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return fmt.Errorf("setting permissions on %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// applyEdit merges one edit into the document and returns the record needed to
// reverse it later.
func applyEdit(doc map[string]any, e configasset.Edit) (configasset.AppliedEdit, error) {
	checksum, err := valueChecksum(e.Value)
	if err != nil {
		return configasset.AppliedEdit{}, err
	}

	applied := configasset.AppliedEdit{
		File:     e.File,
		KeyPath:  e.KeyPath,
		Mode:     e.Mode,
		ID:       e.ID,
		Checksum: checksum,
	}

	switch e.Mode {
	case configasset.ReplaceKey:
		parent, key, err := descend(doc, e.KeyPath, true)
		if err != nil {
			return applied, err
		}
		parent[key] = e.Value

	case configasset.UpsertMapEntry:
		parent, key, err := descend(doc, e.KeyPath, true)
		if err != nil {
			return applied, err
		}
		container, ok := parent[key].(map[string]any)
		if !ok {
			if parent[key] != nil {
				return applied, fmt.Errorf("%s: %q is not an object", e.File, pathString(e.KeyPath))
			}
			container = make(map[string]any)
			parent[key] = container
		}
		container[e.ID] = e.Value

	case configasset.AppendToArray:
		parent, key, err := descend(doc, e.KeyPath, true)
		if err != nil {
			return applied, err
		}
		arr, ok := parent[key].([]any)
		if !ok {
			if parent[key] != nil {
				return applied, fmt.Errorf("%s: %q is not an array", e.File, pathString(e.KeyPath))
			}
			arr = []any{}
		}
		parent[key] = append(arr, e.Value)

	default:
		return applied, fmt.Errorf("unknown merge mode %q", e.Mode)
	}

	return applied, nil
}

// removeEdit reverses an applied edit, taking out exactly the entry tai wrote
// and leaving every sibling alone.
//
// Returns changed=false when there is nothing to remove, and found=false when
// the entry is gone or no longer matches its recorded checksum. A checksum
// mismatch means the user edited the entry after tai installed it, so it is
// left in place - they have taken ownership of it.
func removeEdit(doc map[string]any, a configasset.AppliedEdit) (changed bool, found bool, err error) {
	parent, key, err := descend(doc, a.KeyPath, false)
	if err != nil || parent == nil {
		return false, false, nil
	}

	switch a.Mode {
	case configasset.ReplaceKey:
		current, exists := parent[key]
		if !exists {
			return false, false, nil
		}
		if !checksumMatches(current, a.Checksum) {
			return false, false, nil
		}
		delete(parent, key)
		return true, true, nil

	case configasset.UpsertMapEntry:
		container, ok := parent[key].(map[string]any)
		if !ok {
			return false, false, nil
		}
		current, exists := container[a.ID]
		if !exists {
			return false, false, nil
		}
		if !checksumMatches(current, a.Checksum) {
			return false, false, nil
		}
		delete(container, a.ID)
		if len(container) == 0 {
			delete(parent, key)
		}
		return true, true, nil

	case configasset.AppendToArray:
		arr, ok := parent[key].([]any)
		if !ok {
			return false, false, nil
		}
		for idx, elem := range arr {
			if !checksumMatches(elem, a.Checksum) {
				continue
			}
			parent[key] = append(arr[:idx:idx], arr[idx+1:]...)
			if len(parent[key].([]any)) == 0 {
				delete(parent, key)
				pruneEmptyAncestors(doc, a.KeyPath)
			}
			return true, true, nil
		}
		return false, false, nil

	default:
		return false, false, fmt.Errorf("unknown merge mode %q", a.Mode)
	}
}

// descend walks to the object holding the final key of path, returning that
// object and the key. With create set, missing intermediate objects are made;
// without it, a missing intermediate yields a nil parent.
func descend(doc map[string]any, path []string, create bool) (map[string]any, string, error) {
	if len(path) == 0 {
		return nil, "", fmt.Errorf("empty key path")
	}

	current := doc
	for _, segment := range path[:len(path)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			if current[segment] != nil {
				return nil, "", fmt.Errorf("%q is not an object", segment)
			}
			if !create {
				return nil, "", nil
			}
			next = make(map[string]any)
			current[segment] = next
		}
		current = next
	}

	return current, path[len(path)-1], nil
}

// pruneEmptyAncestors drops intermediate objects along path that tai just
// emptied. It stops at the first non-empty level, so a settings.json whose
// "hooks" key still holds another tool's entries keeps it.
func pruneEmptyAncestors(doc map[string]any, path []string) {
	for i := len(path) - 1; i > 0; i-- {
		parent, key, err := descend(doc, path[:i], false)
		if err != nil || parent == nil {
			return
		}
		obj, ok := parent[key].(map[string]any)
		if !ok || len(obj) > 0 {
			return
		}
		delete(parent, key)
	}
}

// valueChecksum hashes an entry's canonical JSON form, giving a stable
// identity for an entry that is not addressable by key - an element somewhere
// in a shared array.
func valueChecksum(v any) (string, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("hashing config entry: %w", err)
	}
	return ComputeChecksum(encoded), nil
}

// checksumMatches reports whether a document value still hashes to want.
func checksumMatches(v any, want string) bool {
	got, err := valueChecksum(v)
	if err != nil {
		return false
	}
	return got == want
}

// pathString renders a key path for error messages.
func pathString(path []string) string {
	out := ""
	for i, segment := range path {
		if i > 0 {
			out += "."
		}
		out += segment
	}
	return out
}
