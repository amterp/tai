// Package config provides types and parsing for tai configuration files.
package config

import (
	"fmt"
	"io/fs"

	"gopkg.in/yaml.v3"
)

// TaiConfig represents the top-level tai.yaml configuration file.
type TaiConfig struct {
	Version        int      `yaml:"version"`
	Sources        []Source `yaml:"sources"`
	DefaultProfile string   `yaml:"default_profile"`
}

// Source represents a location that provides assets.
type Source struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // "local" (default) or "tai-repo"
	Path string `yaml:"path"`
}

// AssetEntry represents a structured asset reference in a bundle or profile.
type AssetEntry struct {
	Type string `yaml:"type"` // Singular: "persona", "delegate", "skill", "doc"
	Path string `yaml:"path"` // "source:path" or just "path" (defaults to "local" source)
}

// UnmarshalYAML detects the old string-based asset format and provides a clear migration error.
func (ae *AssetEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return fmt.Errorf("asset entry %q must be a mapping with 'type' and 'path' fields, e.g.:\n"+
			"  - type: skill\n"+
			"    path: %s", node.Value, node.Value)
	}

	// Avoid infinite recursion by decoding into a plain struct.
	type raw AssetEntry
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}
	*ae = AssetEntry(r)
	return nil
}

// Bundle represents a named collection of assets.
type Bundle struct {
	Version     int          `yaml:"version,omitempty"` // Schema version (optional, defaults to 1)
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Bundles     []string     `yaml:"bundles"` // Included bundles (bundle names)
	Assets      []AssetEntry `yaml:"assets"`  // Structured asset entries
}

// Profile represents a bundle bound to a specific target.
type Profile struct {
	Version     int            `yaml:"version,omitempty"` // Schema version (optional, defaults to 1)
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Target      string         `yaml:"target"` // e.g., "claude", "cursor"
	Bundles     []string       `yaml:"bundles"`
	Assets      []AssetEntry   `yaml:"assets"`  // Direct asset references
	Options     map[string]any `yaml:"options"` // Adapter-specific options
}

// Asset represents an atomic configuration unit.
type Asset struct {
	Type       string // "persona", "delegate", "skill", "doc"
	Name       string // Filename or directory name
	SourceName string // Source this asset came from
	SourcePath string // Full filesystem path to the asset
	IsDir      bool   // True for skills (which are directories)
}

// ResolvedAsset is an Asset with its content loaded and ready for installation.
//
// The install pipeline populates Content and Metadata as follows:
//   - Content holds the file body AFTER tai frontmatter has been stripped.
//     The adapter's TransformContent receives this stripped body and may
//     prepend target-specific frontmatter (e.g., Claude's name/description/tools).
//     After TransformContent, Content holds the final bytes to write to disk.
//   - Metadata holds the parsed tai frontmatter fields (name, description, tools).
//     The adapter reads Metadata to produce target frontmatter.
//     Unknown fields are silently ignored during validation.
//
// For directory assets (skills), Content and Metadata are nil; the directory is
// copied and SKILL.md is transformed separately via transformSkillMd.
type ResolvedAsset struct {
	Asset
	Content     []byte            // Body after tai frontmatter stripped; final installed bytes after transform
	Metadata    map[string]string // Parsed tai frontmatter fields (nil for directories)
	TargetPath  string            // Where this should be installed (target-relative)
	Checksum    string            // SHA-256 checksum of final content
	BackupPath  string            // Path to backup file (for {{existing}} templated files)
	IsTemplated bool              // Whether this asset uses templating
	FileMode    fs.FileMode       // Source file permissions (zero means default 0644)
}

// AssetType constants (singular form).
const (
	AssetTypePersona    = "persona"
	AssetTypeDelegate   = "delegate"
	AssetTypeSkill      = "skill"
	AssetTypeDoc        = "doc"
	AssetTypeStatusLine = "statusline"
)

// IsConfigAssetType returns true for asset types that are installed as config
// entries in target settings files (rather than as standalone files).
func IsConfigAssetType(t string) bool {
	switch t {
	case AssetTypeStatusLine:
		return true
	default:
		return false
	}
}

// ValidAssetTypes returns all valid asset type names (singular).
func ValidAssetTypes() []string {
	return []string{AssetTypePersona, AssetTypeDelegate, AssetTypeSkill, AssetTypeDoc, AssetTypeStatusLine}
}

// IsValidAssetType checks if the given type is valid.
func IsValidAssetType(t string) bool {
	for _, valid := range ValidAssetTypes() {
		if t == valid {
			return true
		}
	}
	return false
}

// AssetTypeDirName maps a singular asset type to its directory name.
func AssetTypeDirName(t string) string {
	switch t {
	case AssetTypePersona:
		return "personas"
	case AssetTypeDelegate:
		return "delegates"
	case AssetTypeSkill:
		return "skills"
	case AssetTypeDoc:
		return "docs"
	case AssetTypeStatusLine:
		return "statuslines"
	default:
		return t
	}
}

// AssetTypeFromDirName maps a directory name back to its singular asset type.
func AssetTypeFromDirName(d string) string {
	switch d {
	case "personas":
		return AssetTypePersona
	case "delegates":
		return AssetTypeDelegate
	case "skills":
		return AssetTypeSkill
	case "docs":
		return AssetTypeDoc
	case "statuslines":
		return AssetTypeStatusLine
	default:
		return d
	}
}

// StatusLineConfig represents a parsed status line configuration asset.
// This is the tai-agnostic format that gets transformed by adapters.
type StatusLineConfig struct {
	// Level controls the status line style: minimal, standard, detailed, hidden, or custom.
	// This field is required for tai format detection.
	Level string `yaml:"level"`

	// Show lists elements to display when Level is minimal/standard/detailed.
	// Valid values: cost, model, tokens, latency, context
	Show []string `yaml:"show,omitempty"`

	// Command is the custom command path (required when Level is "custom").
	Command string `yaml:"command,omitempty"`

	// Padding is optional padding value (only for Level "custom").
	Padding *int `yaml:"padding,omitempty"`
}

// IsTaiFormat returns true if this config appears to be in tai format.
// Tai format requires the "level" field; configs without it are treated as native.
func (c *StatusLineConfig) IsTaiFormat() bool {
	return c.Level != ""
}

// Valid status line levels.
const (
	StatusLineLevelMinimal  = "minimal"
	StatusLineLevelStandard = "standard"
	StatusLineLevelDetailed = "detailed"
	StatusLineLevelHidden   = "hidden"
	StatusLineLevelCustom   = "custom"
)

// Valid status line show elements.
var ValidStatusLineShowElements = []string{"cost", "model", "tokens", "latency", "context"}
