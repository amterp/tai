// Package config provides types and parsing for tai configuration files.
package config

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

// Bundle represents a named collection of assets.
type Bundle struct {
	Version     int      `yaml:"version,omitempty"` // Schema version (optional, defaults to 1)
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Bundles     []string `yaml:"bundles"` // Included bundles (bundle names)
	Assets      []string `yaml:"assets"`  // Asset paths (source:path or just path for local)
}

// Profile represents a bundle bound to a specific target.
type Profile struct {
	Version     int               `yaml:"version,omitempty"` // Schema version (optional, defaults to 1)
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Target      string            `yaml:"target"` // e.g., "claude", "cursor"
	Bundles     []string          `yaml:"bundles"`
	Assets      []string          `yaml:"assets"`  // Direct asset references
	Options     map[string]any    `yaml:"options"` // Adapter-specific options
}

// Asset represents an atomic configuration unit.
type Asset struct {
	Type       string // "personas", "delegates", "skills", "docs"
	Name       string // Filename or directory name
	SourceName string // Source this asset came from
	SourcePath string // Full filesystem path to the asset
	IsDir      bool   // True for skills (which are directories)
}

// ResolvedAsset is an Asset with its content loaded and ready for installation.
type ResolvedAsset struct {
	Asset
	Content     []byte // File content (nil for directories)
	TargetPath  string // Where this should be installed (target-relative)
	Checksum    string // SHA-256 checksum of content
	BackupPath  string // Path to backup file (for {{existing}} templated files)
	IsTemplated bool   // Whether this asset uses templating
}

// AssetType constants.
const (
	AssetTypePersonas  = "personas"
	AssetTypeDelegates = "delegates"
	AssetTypeSkills    = "skills"
	AssetTypeDocs      = "docs"
)

// ValidAssetTypes returns all valid asset type names.
func ValidAssetTypes() []string {
	return []string{AssetTypePersonas, AssetTypeDelegates, AssetTypeSkills, AssetTypeDocs}
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
