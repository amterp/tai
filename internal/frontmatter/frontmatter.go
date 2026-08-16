// Package frontmatter handles parsing and rendering of YAML frontmatter
// delimited by "---" markers. It is target-agnostic.
package frontmatter

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Field name constants for tai frontmatter.
const (
	FieldName        = "name"
	FieldDescription = "description"
	FieldTools       = "tools"
	FieldModel       = "model"
	FieldDepends     = "depends"
	FieldNative      = "native"
)

// taiFields is every key tai consumes from frontmatter. Anything else is
// parsed and then dropped, which the installer warns about.
var taiFields = map[string]bool{
	FieldName:        true,
	FieldDescription: true,
	FieldTools:       true,
	FieldModel:       true,
	FieldDepends:     true,
	FieldNative:      true,
}

// IsTaiField reports whether a frontmatter key is part of tai's vocabulary.
func IsTaiField(key string) bool {
	return taiFields[key]
}

var separator = []byte("---")

// DependencyEntry represents a single dependency in the depends array.
// The path format is: "type-dir/name" or "source:type-dir/name"
// Examples: "delegates/bug-hunter.md", "other-source:skills/my-skill"
type DependencyEntry struct {
	Path string // Full asset path including type directory
}

// Parsed represents a file split into frontmatter metadata and body content.
type Parsed struct {
	Metadata map[string]string // Parsed YAML key-value pairs (nil if no frontmatter)
	Depends  []DependencyEntry // Parsed depends array (nil if not present)
	Native   Native            // Parsed native block (nil if not present)
	Body     []byte            // Content after the frontmatter block
}

// Native holds per-target frontmatter fields, keyed by target name. Its
// contents are whatever the target's own format calls for; tai does not
// interpret them beyond merging them into what the adapter rendered.
type Native map[string]map[string]any

// Parse separates YAML frontmatter from body content.
// If the content doesn't start with "---", Metadata is nil and Body is the full content.
func Parse(content []byte) (*Parsed, error) {
	trimmed := bytes.TrimLeftFunc(content, unicode.IsSpace)
	if !bytes.HasPrefix(trimmed, separator) {
		return &Parsed{Body: content}, nil
	}

	// Find the closing "---"
	afterOpen := trimmed[len(separator):]
	// Skip the newline after opening ---
	if len(afterOpen) > 0 && afterOpen[0] == '\n' {
		afterOpen = afterOpen[1:]
	} else if len(afterOpen) > 1 && afterOpen[0] == '\r' && afterOpen[1] == '\n' {
		afterOpen = afterOpen[2:]
	}

	var yamlBlock, rest []byte
	if bytes.HasPrefix(afterOpen, separator) {
		// Empty frontmatter: closing --- immediately follows opening
		yamlBlock = nil
		rest = afterOpen[len(separator):]
	} else {
		closeIdx := bytes.Index(afterOpen, append([]byte("\n"), separator...))
		if closeIdx == -1 {
			// No closing delimiter — treat as no frontmatter
			return &Parsed{Body: content}, nil
		}
		yamlBlock = afterOpen[:closeIdx]
		rest = afterOpen[closeIdx+1+len(separator):] // skip \n---
	}

	// Skip the newline after closing ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	// Skip the blank separator line between frontmatter and body (if present).
	// Render re-adds this, so stripping it here enables clean round-tripping.
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	// Parse as map[string]any first to handle the depends array
	var rawMetadata map[string]any
	if err := yaml.Unmarshal(yamlBlock, &rawMetadata); err != nil {
		return nil, improveYAMLError(err)
	}

	metadata := make(map[string]string)
	var depends []DependencyEntry
	var native Native

	for key, val := range rawMetadata {
		switch key {
		case FieldDepends:
			deps, err := parseDepends(val)
			if err != nil {
				return nil, fmt.Errorf("parsing depends: %w", err)
			}
			depends = deps
		case FieldNative:
			n, err := parseNative(val)
			if err != nil {
				return nil, fmt.Errorf("parsing native: %w", err)
			}
			native = n
		default:
			// All other fields are strings
			switch v := val.(type) {
			case string:
				metadata[key] = v
			case nil:
				// Skip nil values
			default:
				// Convert non-string scalars to string representation
				metadata[key] = fmt.Sprintf("%v", v)
			}
		}
	}

	return &Parsed{Metadata: metadata, Depends: depends, Native: native, Body: rest}, nil
}

// parseNative parses the native field into per-target field maps.
//
// A malformed native block is an error rather than a silent skip: its whole
// purpose is carrying fields tai does not understand, so tai cannot tell a
// wrong shape from a field it merely does not recognize by looking at the
// installed file later.
func parseNative(val any) (Native, error) {
	raw, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be a map of target name to fields (e.g. \"native:\\n  claude:\\n    key: value\"), got %T", val)
	}

	native := make(Native, len(raw))
	for target, fields := range raw {
		if fields == nil {
			continue
		}
		fieldMap, ok := fields.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("native.%s: must be a map of fields, got %T", target, fields)
		}
		native[target] = fieldMap
	}

	if len(native) == 0 {
		return nil, nil
	}
	return native, nil
}

// parseDepends parses the depends field value into a slice of DependencyEntry.
// Each entry should be a string path like "delegates/bug-hunter.md" or "source:skills/my-skill".
func parseDepends(val any) ([]DependencyEntry, error) {
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("depends must be an array, got %T", val)
	}

	var deps []DependencyEntry
	for i, item := range arr {
		pathStr, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("depends[%d]: must be a string path (e.g., \"delegates/bug-hunter.md\"), got %T", i, item)
		}

		if pathStr == "" {
			return nil, fmt.Errorf("depends[%d]: empty dependency path", i)
		}

		deps = append(deps, DependencyEntry{Path: pathStr})
	}

	return deps, nil
}

// Fields is the frontmatter an adapter is building, in the order it will be
// written. Order is part of the output: rendered bytes are checksummed, and a
// checksum that moves with Go's map iteration would report a phantom conflict
// on every install.
type Fields struct {
	keys   []string
	values map[string]any
}

// Set adds a field, or replaces one already present without moving it.
func (f *Fields) Set(key string, val any) {
	if f.values == nil {
		f.values = make(map[string]any)
	}
	if _, exists := f.values[key]; !exists {
		f.keys = append(f.keys, key)
	}
	f.values[key] = val
}

// SetString adds a string field, skipping empty values. Adapters use this for
// optional fields, where an empty string means the source did not set one.
func (f *Fields) SetString(key, val string) {
	if val == "" {
		return
	}
	f.Set(key, val)
}

// Merge overlays a target's native fields, which win over what the adapter
// derived from tai's own fields. New keys append in sorted order so the
// result does not depend on map iteration.
func (f *Fields) Merge(native map[string]any) {
	for _, key := range sortedKeys(native) {
		f.Set(key, native[key])
	}
}

// Len reports how many fields will be rendered.
func (f *Fields) Len() int {
	return len(f.keys)
}

// Render produces content with YAML frontmatter followed by the body.
// Values are properly escaped using the yaml.v3 encoder.
func Render(fields Fields, body []byte) []byte {
	content := make([]*yaml.Node, 0, len(fields.keys)*2)
	for _, key := range fields.keys {
		content = append(content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			valueNode(fields.values[key]),
		)
	}

	// Encode the mapping node to YAML
	node := &yaml.Node{Kind: yaml.MappingNode, Content: content}
	var yamlBuf bytes.Buffer
	enc := yaml.NewEncoder(&yamlBuf)
	enc.SetIndent(2)
	_ = enc.Encode(node) // errors only on invalid node structure, which we control
	_ = enc.Close()

	// Assemble final output with --- delimiters
	var buf bytes.Buffer
	buf.Write(separator)
	buf.WriteByte('\n')
	buf.Write(yamlBuf.Bytes())
	buf.Write(separator)
	buf.WriteByte('\n')

	if len(body) > 0 {
		buf.WriteByte('\n')
		buf.Write(body)
	}

	return buf.Bytes()
}

// valueNode encodes a field value for the frontmatter mapping.
//
// Strings go through an untagged scalar node so the encoder decides quoting
// for itself - that is what quotes a description containing ": ". Everything
// else (native fields can be lists, maps, booleans) is encoded normally.
func valueNode(val any) *yaml.Node {
	if s, ok := val.(string); ok {
		return &yaml.Node{Kind: yaml.ScalarNode, Value: s}
	}
	node := &yaml.Node{}
	if err := node.Encode(val); err != nil {
		// A value YAML cannot represent - a channel or a func - can only come
		// from tai's own code, never from a parsed file.
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%v", val)}
	}
	return node
}

// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// improveYAMLError wraps common YAML errors with actionable suggestions.
func improveYAMLError(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "mapping values are not allowed in this context") {
		return fmt.Errorf("%w\n\nHint: If your description contains colons (e.g., \"user: ...\"), use a block scalar:\n  description: >-\n    Your description text here", err)
	}
	return err
}
