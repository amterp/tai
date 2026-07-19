// Package config provides types and parsing for tai configuration files.
package config

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// TaiConfig represents the top-level tai.yaml configuration file.
type TaiConfig struct {
	Version        int               `yaml:"version"`
	Vars           map[string]string `yaml:"vars"`
	Sources        []Source          `yaml:"sources"`
	DefaultProfile string            `yaml:"default_profile"`
}

// Source represents a location that provides assets.
type Source struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // "local" (default), "tai-repo", or "command"
	Path string `yaml:"path"` // local/tai-repo only

	// Command materializes the source ("command" type only). The argv list is
	// executed with a tai-provided staging directory (via the TAI_DEST_DIR env
	// var); any files it writes there are atomically swapped into the source's
	// cache directory, which is then treated exactly like a local source.
	Command []string `yaml:"command"`

	// Timeout for the command, parsed via time.ParseDuration. Defaults to
	// DefaultCommandTimeout when unset. Command sources only.
	Timeout string `yaml:"timeout"`

	// Patches is a config-repo-relative directory of patch files applied to
	// this source's content. Declared here rather than by convention so the
	// source and its local delta are visible in one place.
	Patches string `yaml:"patches"`

	// Exclude lists glob patterns for files this source does not carry at all.
	// Deliberately independent of Patches: a patch says "tell me when upstream
	// touches these lines", an exclusion says "upstream can do what it likes
	// with this, I'm not taking it" - and filtering an import is useful on its
	// own. Patterns are source-relative and matched per path segment; see
	// IsExcluded for what a pattern can express.
	Exclude []string `yaml:"exclude"`
}

// HasPatches reports whether this source declares a patch directory.
func (s *Source) HasPatches() bool {
	return s.Patches != ""
}

// IsExcluded reports whether a source-relative path is excluded.
//
// A pattern matches the path itself or any of its parent directories, so
// `skills/legacy-*` excludes that directory and everything beneath it - the
// behavior people expect from excluding a directory, without needing `**`. A
// trailing `/**` is accepted and stripped for familiarity.
//
// Wildcards are `path.Match`, so they do not span separators: `skills/*` matches
// `skills/rad` but not `skills/a/b`. Subtree exclusion comes from the
// parent-directory walk above, not from a wildcard. An interior `**` cannot
// match anything and is rejected when the config is parsed, rather than being
// accepted and silently carrying the file it was meant to drop.
func (s *Source) IsExcluded(rel string) bool {
	if len(s.Exclude) == 0 {
		return false
	}
	rel = path.Clean(filepath.ToSlash(rel))
	for _, pattern := range s.Exclude {
		pattern = strings.TrimSuffix(path.Clean(filepath.ToSlash(pattern)), "/**")
		for candidate := rel; candidate != "." && candidate != "/"; candidate = path.Dir(candidate) {
			if ok, err := path.Match(pattern, candidate); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// SourceTypeCommand is the source type whose assets are materialized by
// running a user-declared command.
const SourceTypeCommand = "command"

// IsCommand returns true if this source is materialized by running a command.
func (s *Source) IsCommand() bool {
	return s.Type == SourceTypeCommand
}

// ResolvedTimeout returns the parsed command timeout, or DefaultCommandTimeout
// when unset. Validation guarantees the string parses cleanly; if validation
// was skipped and it's malformed, the default is returned instead.
func (s *Source) ResolvedTimeout() time.Duration {
	return resolveTimeout(s.Timeout)
}

// AssetEntry represents a structured asset reference in a bundle or profile.
type AssetEntry struct {
	Type string `yaml:"type"` // Singular: "persona", "delegate", "skill", "file", "statusline"
	Path string `yaml:"path"` // "source:path" or just "path" (defaults to "local" source)
	Dest string `yaml:"dest"` // Target destination path (required for "file" type only)
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
	Version     int               `yaml:"version,omitempty"` // Schema version (optional, defaults to 1)
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Target      string            `yaml:"target"` // e.g., "claude", "opencode"
	Bundles     []string          `yaml:"bundles"`
	Assets      []AssetEntry      `yaml:"assets"`  // Direct asset references
	Options     map[string]any    `yaml:"options"` // Adapter-specific options
	Vars        map[string]string `yaml:"vars"`    // Template variables (overrides tai.yaml vars)

	// PostInstall is an argv list run after this profile installs
	// successfully. It runs for side effects (notifications, follow-up
	// tooling); a failure does not roll the install back, but is reported
	// and makes tai exit nonzero.
	PostInstall []string `yaml:"post_install"`

	// PostInstallTimeout bounds the hook, parsed via time.ParseDuration.
	// Defaults to DefaultCommandTimeout when unset.
	PostInstallTimeout string `yaml:"post_install_timeout"`
}

// ResolvedPostInstallTimeout returns the parsed post-install hook timeout, or
// DefaultCommandTimeout when unset.
func (p *Profile) ResolvedPostInstallTimeout() time.Duration {
	return resolveTimeout(p.PostInstallTimeout)
}

// Asset represents an atomic configuration unit.
type Asset struct {
	Type       string // "persona", "delegate", "skill", "file", "statusline"
	Name       string // Filename or directory name
	SourceName string // Source this asset came from
	SourcePath string // Full filesystem path to the asset
	IsDir      bool   // True for directory assets (any type can be a directory)
	Dest       string // Target destination path (used by "file" type)
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
	AssetTypeFile       = "file"
	AssetTypeStatusLine = "statusline"
)

// MainFileName constants for directory assets.
// Each directory asset type has a conventional main file.
const (
	MainFilePersona    = "PERSONA.md"
	MainFileDelegate   = "DELEGATE.md"
	MainFileSkill      = "SKILL.md"
	MainFileStatusLine = "config.yaml"
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
	return []string{AssetTypePersona, AssetTypeDelegate, AssetTypeSkill, AssetTypeFile, AssetTypeStatusLine}
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
	case AssetTypeFile:
		return "files"
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
	case "files":
		return AssetTypeFile
	case "statuslines":
		return AssetTypeStatusLine
	default:
		return d
	}
}

// MainFileName returns the main file name for a directory asset type.
// Returns empty string for types that don't support directory mode.
// Note: StatusLine directory support is not yet implemented, so it's excluded here.
func MainFileName(t string) string {
	switch t {
	case AssetTypePersona:
		return MainFilePersona
	case AssetTypeDelegate:
		return MainFileDelegate
	case AssetTypeSkill:
		return MainFileSkill
	default:
		return ""
	}
}

// SupportsDirectory returns true if the asset type can be a directory.
func SupportsDirectory(t string) bool {
	return MainFileName(t) != ""
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
