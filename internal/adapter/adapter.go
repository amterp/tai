// Package adapter provides the interface for target-specific adapters.
package adapter

import (
	"fmt"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/template"
)

// Adapter defines the interface for target-specific adapters.
type Adapter interface {
	// Name returns the adapter name (e.g., "claude", "opencode")
	Name() string

	// AssetMapping returns how tai asset types map to target paths.
	// e.g., "personas" -> "output-styles", "delegates" -> "agents"
	AssetMapping() map[string]string

	// TargetDir returns the base directory where this target's config lives.
	// e.g., ~/.claude
	TargetDir() string

	// MapAssetPath maps a tai asset path to a target path.
	// e.g., "personas/senior-swe.md" -> "output-styles/senior-swe.md"
	MapAssetPath(asset *config.Asset) string

	// TransformContent transforms a resolved asset's content into the target's
	// expected format. For example, the Claude adapter converts tai frontmatter
	// into Claude-specific frontmatter for personas and delegates.
	// The asset's Content holds the body (frontmatter already stripped) and
	// Metadata holds the parsed frontmatter fields.
	TransformContent(asset *config.ResolvedAsset) ([]byte, error)

	// Run invokes the target with the given arguments.
	Run(args []string) error

	// SupportedConfigAssetTypes returns the config asset types this adapter
	// supports. Config assets are installed as entries in target settings files
	// rather than as standalone files. Returns an empty slice if none are.
	//
	// Every type listed here must also have its translator interface
	// implemented - StatusLineTranslator, HookTranslator, MCPTranslator - or
	// installing that type fails at runtime. Nothing enforces that pairing at
	// compile time; the invariant is covered by tests instead.
	SupportedConfigAssetTypes() []string

	// InstallWarnings returns non-fatal, adapter-specific warnings for an asset.
	// This allows adapters to communicate fallback behavior during install.
	InstallWarnings(asset *config.Asset) []string

	// ConfigFilePath returns the config file a given asset type installs into
	// at a given scope ("user", "project", "local"). The asset type is part of
	// the question, not just the scope: a target may keep different config
	// asset types in different files - Claude holds status lines and hooks in
	// settings.json but MCP servers elsewhere.
	//
	// Returns empty string when this target has no such file for that
	// type and scope, which the installer reports as unsupported.
	ConfigFilePath(assetType, scope string) string

	// SettingsKey returns the JSON key name for a config asset type in the target's
	// settings file. For example, Claude uses "statusLine" for statusline configs.
	// Returns empty string if the asset type doesn't map to a settings key.
	SettingsKey(assetType string) string

	// MainFileName returns the main file name for a directory asset type.
	// For example, Claude returns "SKILL.md" for skills, "PERSONA.md" for personas.
	// Returns empty string for types that don't support directory mode.
	MainFileName(assetType string) string

	// ProvideVariables returns target-specific template variables.
	// These variables use the "target_" prefix convention:
	//   - target_name: adapter name (e.g., "claude", "opencode")
	//   - target_agents_file: global instructions filename (e.g., "CLAUDE.md")
	// User-provided variables (via --var) take precedence over these.
	ProvideVariables() template.Variables

	// UnsupportedFileAssetTypes returns file asset types this adapter does not
	// support at all. These assets will be skipped during install with a warning.
	// This differs from InstallWarnings, which warns but still installs.
	// Returns nil if all file asset types are supported.
	UnsupportedFileAssetTypes() []string
}

// Registry holds all registered adapters.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry creates a new adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an adapter to the registry.
func (r *Registry) Register(adapter Adapter) {
	r.adapters[adapter.Name()] = adapter
}

// Get returns an adapter by name.
func (r *Registry) Get(name string) (Adapter, error) {
	adapter, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("unknown target: %s", name)
	}
	return adapter, nil
}

// List returns all registered adapter names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is the global registry with all built-in adapters.
// Adapters register themselves via init().
var DefaultRegistry = NewRegistry()
