// Package configasset models the config asset types - assets that install as
// entries inside a target's config files rather than as standalone files.
//
// The central idea is that tai owns *entries*, not *keys*. A target's config
// file is shared: `settings.json` holds hooks written by tai next to hooks
// written by the user and by unrelated tools, and `~/.claude.json` holds MCP
// servers next to a session's entire mutable state. So a handler never says
// "write this file"; it returns a list of Edits describing exactly which
// entries it wants, and the installer merges them into the parsed document and
// records enough in the manifest to take precisely those entries back out
// again.
package configasset

import (
	"fmt"
	"sort"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
)

// Cardinality says how many entries of a type may coexist on one target.
type Cardinality int

const (
	// Singleton types own a single entry; a second asset of the type overrides
	// the first. A status line is the example - a target has exactly one.
	Singleton Cardinality = iota

	// Collection types hold many entries keyed by asset name. Hooks and MCP
	// servers are collections: a profile carries as many as it likes, and they
	// coexist rather than overriding.
	Collection
)

// MergeMode describes how an Edit combines with whatever is already at its
// KeyPath. The mode also determines how the edit is reversed on uninstall.
type MergeMode string

const (
	// ReplaceKey sets the value at KeyPath outright, and removal deletes the
	// key. Only safe where tai owns the whole key, as with a status line.
	ReplaceKey MergeMode = "replace_key"

	// UpsertMapEntry sets KeyPath[ID] within an object, leaving sibling keys
	// alone. Removal deletes just that one key. This is how MCP servers merge.
	UpsertMapEntry MergeMode = "upsert_map_entry"

	// AppendToArray appends to the array at KeyPath, leaving existing elements
	// alone. Removal deletes the single element matching the recorded
	// checksum. This is how hooks merge, since an event's array is shared with
	// entries tai did not write.
	AppendToArray MergeMode = "append_to_array"
)

// Edit is one declarative mutation of one target config file.
type Edit struct {
	// File is the absolute path of the config file to modify.
	File string

	// KeyPath locates the entry, e.g. ["statusLine"], ["mcpServers"], or
	// ["hooks", "PreToolUse"]. Missing intermediate objects are created.
	KeyPath []string

	// Mode selects merge and removal semantics.
	Mode MergeMode

	// ID names the entry within its container. Required for UpsertMapEntry
	// (the map key); unused by the other modes.
	ID string

	// Value is the entry itself, ready to serialize.
	Value any
}

// AppliedEdit is the manifest record of an Edit that was actually written. It
// carries a checksum of the value rather than the value itself: enough to find
// the entry again for removal, and enough to notice the user has since edited
// it, without bloating the manifest with a copy of every installed entry.
type AppliedEdit struct {
	File     string    `yaml:"file"`
	KeyPath  []string  `yaml:"key_path"`
	Mode     MergeMode `yaml:"mode"`
	ID       string    `yaml:"id,omitempty"`
	Checksum string    `yaml:"checksum"`
}

// Request is the input to a handler: one config asset, already templated,
// bound to a target and scope.
type Request struct {
	// Adapter is the target being installed to. Handlers use it for config
	// file paths and settings keys, and type-assert it to the optional
	// translator interfaces for target-specific vocabulary.
	Adapter adapter.Adapter

	// Asset is the source asset, for error messages and provenance.
	Asset *config.Asset

	// Content is the asset file's bytes after template processing.
	Content []byte

	// Scope is the resolved installation scope ("user", "project", "local").
	Scope string

	// Name is the entry's identity within its type - the asset filename with
	// its extension stripped.
	Name string
}

// Plan is a handler's output: the edits to apply, plus any non-fatal warnings
// worth showing the user (a field or event this target cannot express).
type Plan struct {
	Edits    []Edit
	Warnings []string
}

// Handler owns one config asset type: parsing and validating its YAML, and
// translating it into edits for a given target.
type Handler interface {
	// Type returns the asset type name, e.g. "statusline".
	Type() string

	// Cardinality reports whether many entries of this type may coexist.
	Cardinality() Cardinality

	// Plan parses and validates the asset, then returns the edits that install
	// it. An error here aborts the install; use Plan.Warnings for degradation
	// that should not be fatal.
	Plan(req Request) (*Plan, error)
}

// registry holds the handler for each config asset type.
var registry = map[string]Handler{}

// Register adds a handler. It panics on a duplicate type, which can only be a
// programming error at init time.
func Register(h Handler) {
	if _, exists := registry[h.Type()]; exists {
		panic(fmt.Sprintf("configasset: handler already registered for type %q", h.Type()))
	}
	registry[h.Type()] = h
}

// Lookup returns the handler for a config asset type.
func Lookup(assetType string) (Handler, bool) {
	h, ok := registry[assetType]
	return h, ok
}

// Types returns every registered config asset type, sorted for stable output.
func Types() []string {
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// IsCollection reports whether a type may hold many entries. Unregistered
// types report false, matching the conservative singleton behavior tai had
// before collections existed.
func IsCollection(assetType string) bool {
	h, ok := Lookup(assetType)
	return ok && h.Cardinality() == Collection
}
