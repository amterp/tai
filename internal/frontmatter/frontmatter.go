// Package frontmatter handles parsing and rendering of YAML frontmatter
// delimited by "---" markers. It is target-agnostic.
package frontmatter

import (
	"bytes"
	"fmt"
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
)

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
	Body     []byte            // Content after the frontmatter block
}

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

	for key, val := range rawMetadata {
		if key == FieldDepends {
			// Parse depends array
			deps, err := parseDepends(val)
			if err != nil {
				return nil, fmt.Errorf("parsing depends: %w", err)
			}
			depends = deps
		} else {
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

	return &Parsed{Metadata: metadata, Depends: depends, Body: rest}, nil
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

// Render produces content with YAML frontmatter followed by the body.
// fieldOrder controls the order fields appear in the frontmatter block.
// Fields in the map but not in fieldOrder are appended alphabetically.
// Values are properly escaped using the yaml.v3 encoder.
func Render(fields map[string]string, fieldOrder []string, body []byte) []byte {
	// Build ordered content slice for the mapping node
	var content []*yaml.Node
	written := make(map[string]bool)

	for _, key := range fieldOrder {
		if val, ok := fields[key]; ok {
			content = append(content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: key},
				&yaml.Node{Kind: yaml.ScalarNode, Value: val},
			)
			written[key] = true
		}
	}

	for _, key := range sortedKeys(fields) {
		if written[key] {
			continue
		}
		content = append(content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Value: fields[key]},
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


// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort — these maps are tiny
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
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
