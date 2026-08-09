// Package opencode provides the adapter for OpenCode.
package opencode

import (
	"os"
	"path/filepath"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
	fm "github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/template"
)

// fieldMode is the OpenCode-specific frontmatter field that distinguishes
// primary agents from subagents. Not a tai concept - only meaningful to OpenCode.
const fieldMode = "mode"

func init() {
	adapter.DefaultRegistry.Register(New(""))
}

// Adapter implements the adapter.Adapter interface for OpenCode.
type Adapter struct {
	targetDir string // Override for testing
}

// New creates a new OpenCode adapter.
// If targetDir is empty, defaults to $XDG_CONFIG_HOME/opencode or ~/.config/opencode.
func New(targetDir string) *Adapter {
	if targetDir == "" {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			targetDir = filepath.Join(xdg, "opencode")
		}
	}
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			targetDir = filepath.Join(".config", "opencode") // Fallback
		} else {
			targetDir = filepath.Join(home, ".config", "opencode")
		}
	}
	return &Adapter{targetDir: targetDir}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "opencode"
}

// AssetMapping returns how tai asset types map to OpenCode paths.
// Both personas and delegates map to "agents/" - OpenCode unifies these
// into a single agent concept, distinguished by the mode field.
func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypePersona:  "agents",
		config.AssetTypeDelegate: "agents",
		config.AssetTypeSkill:    "skills",
		// Note: "file" type uses asset.Dest directly, not this mapping
	}
}

// TargetDir returns the OpenCode config directory.
func (a *Adapter) TargetDir() string {
	return a.targetDir
}

// MapAssetPath maps a tai asset to an OpenCode target path.
func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	return adapter.DefaultMapAssetPath(a.AssetMapping(), asset)
}

// TransformContent transforms asset content into OpenCode's expected format.
// Personas get description-only frontmatter (no name - OpenCode uses filename).
// Delegates get description + mode: subagent (no name, no tools).
// Skills get name + description (tools/model omitted - same rationale as Codex).
// Files are returned as-is.
func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	switch asset.Type {
	case config.AssetTypePersona:
		return a.transformPersona(asset), nil
	case config.AssetTypeDelegate:
		return a.transformDelegate(asset), nil
	case config.AssetTypeSkill:
		return a.transformSkill(asset), nil
	case config.AssetTypeFile:
		return asset.Content, nil
	default:
		return asset.Content, nil
	}
}

// resolveDescription returns the description for an OpenCode agent, falling
// back to name if description is absent. OpenCode requires a description
// for agents, so this ensures we always provide something meaningful.
func resolveDescription(metadata map[string]string) string {
	if desc := metadata[fm.FieldDescription]; desc != "" {
		return desc
	}
	return metadata[fm.FieldName]
}

// transformPersona renders OpenCode agent frontmatter for a persona.
// OpenCode agents are named by filename, so name is omitted.
func (a *Adapter) transformPersona(asset *config.ResolvedAsset) []byte {
	desc := resolveDescription(asset.Metadata)
	if desc == "" {
		return asset.Content
	}

	fields := map[string]string{fm.FieldDescription: desc}
	order := []string{fm.FieldDescription}

	return fm.Render(fields, order, asset.Content)
}

// transformDelegate renders OpenCode agent frontmatter for a delegate.
// Like personas, name is omitted (filename-based). Adds mode: subagent
// to distinguish from primary agents. Tools are omitted because OpenCode
// uses a structurally incompatible format (boolean map vs comma string).
func (a *Adapter) transformDelegate(asset *config.ResolvedAsset) []byte {
	desc := resolveDescription(asset.Metadata)

	fields := map[string]string{
		fieldMode: "subagent",
	}
	order := []string{}

	if desc != "" {
		fields[fm.FieldDescription] = desc
		order = append(order, fm.FieldDescription)
	}
	order = append(order, fieldMode)

	if model := asset.Metadata[fm.FieldModel]; model != "" {
		fields[fm.FieldModel] = model
		order = append(order, fm.FieldModel)
	}

	return fm.Render(fields, order, asset.Content)
}

// transformSkill renders OpenCode skill frontmatter (name + description).
// Tools and model are omitted - OpenCode skills use the same minimal
// frontmatter as Codex skills, not the extended Claude format.
func (a *Adapter) transformSkill(asset *config.ResolvedAsset) []byte {
	fields := map[string]string{fm.FieldName: asset.Metadata[fm.FieldName]}
	order := []string{fm.FieldName}

	if desc := asset.Metadata[fm.FieldDescription]; desc != "" {
		fields[fm.FieldDescription] = desc
		order = append(order, fm.FieldDescription)
	}

	return fm.Render(fields, order, asset.Content)
}

// Run invokes opencode with the given arguments.
func (a *Adapter) Run(args []string) error {
	return adapter.RunBinary("opencode", args)
}

// SupportedConfigAssetTypes returns the config asset types OpenCode supports.
func (a *Adapter) SupportedConfigAssetTypes() []string {
	return []string{config.AssetTypeMCP}
}

// InstallWarnings returns OpenCode-specific advisory install warnings.
// OpenCode actively consumes agents and skills, so no warnings needed.
func (a *Adapter) InstallWarnings(_ *config.Asset) []string {
	return nil
}

// MCPSettingsKey is the top-level key OpenCode reads MCP servers from. Note
// it is "mcp", not the "mcpServers" the other targets use.
const MCPSettingsKey = "mcp"

// ConfigFilePath returns the OpenCode config file for an asset type and scope.
func (a *Adapter) ConfigFilePath(assetType, scope string) string {
	if assetType != config.AssetTypeMCP {
		return ""
	}

	switch scope {
	case config.ScopeProject, config.ScopeLocal:
		rel := "opencode.json"
		abs, err := filepath.Abs(rel)
		if err != nil {
			return rel
		}
		return abs
	default: // user
		return filepath.Join(a.targetDir, "opencode.json")
	}
}

// TransformMCP renders an MCP server in OpenCode's shape.
//
// OpenCode diverges structurally from the other targets: the command and its
// arguments are a single argv array, the environment is spelled "environment",
// and "type" is required rather than inferred. Its config schema also rejects
// unknown keys outright, so a field in the wrong place is a hard failure there
// rather than something quietly ignored.
// MCPKeyPath locates the server map. OpenCode keeps every scope's servers
// under one top-level key; only the config file differs.
func (a *Adapter) MCPKeyPath(_ string) []string {
	return []string{MCPSettingsKey}
}

func (a *Adapter) TransformMCP(_ string, cfg *config.MCPConfig) (map[string]any, []string, error) {
	entry := map[string]any{}
	var warnings []string

	switch cfg.Transport {
	case config.MCPTransportStdio:
		entry["type"] = "local"
		command := make([]any, 0, len(cfg.Args)+1)
		command = append(command, cfg.Command)
		for _, arg := range cfg.Args {
			command = append(command, arg)
		}
		entry["command"] = command

		if len(cfg.Env) > 0 {
			env := make(map[string]any, len(cfg.Env))
			for k, v := range cfg.Env {
				env[k] = configasset.RewriteEnvRefs(v, func(name string) string {
					return "{env:" + name + "}"
				})
			}
			entry["environment"] = env
		}
		if cfg.Cwd != "" {
			entry["cwd"] = cfg.Cwd
		}

	case config.MCPTransportHTTP:
		entry["type"] = "remote"
		entry["url"] = cfg.URL
		if len(cfg.Headers) > 0 {
			headers := make(map[string]any, len(cfg.Headers))
			for k, v := range cfg.Headers {
				headers[k] = configasset.RewriteEnvRefs(v, func(name string) string {
					return "{env:" + name + "}"
				})
			}
			entry["headers"] = headers
		}
	}

	if cfg.Description != "" {
		// OpenCode validates additionalProperties: false, so an unrecognized
		// key would make the whole config invalid rather than be ignored.
		warnings = append(warnings,
			"OpenCode MCP entries have no 'description' field - dropped")
	}

	for key, value := range cfg.Native[a.Name()] {
		entry[key] = value
	}

	return entry, warnings, nil
}

// SettingsKey returns the JSON key name for a config asset type.
func (a *Adapter) SettingsKey(assetType string) string {
	if assetType == config.AssetTypeMCP {
		return MCPSettingsKey
	}
	return ""
}

// MainFileName returns the main file name for a directory asset type.
func (a *Adapter) MainFileName(assetType string) string {
	return config.MainFileName(assetType)
}

// UnsupportedFileAssetTypes returns nil - OpenCode supports all file asset types.
func (a *Adapter) UnsupportedFileAssetTypes() []string {
	return nil
}

// ProvideVariables returns OpenCode-specific template variables.
func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         "opencode",
		"target_name_display": "OpenCode",
		"target_agents_file":  "AGENTS.md",
	}
}
