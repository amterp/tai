// Package claude provides the adapter for Claude Code.
package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	fm "github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/template"
)

// Settings key constants for Claude's settings.json
const (
	StatusLineSettingsKey = "statusLine"
)

func init() {
	adapter.DefaultRegistry.Register(New(""))
}

// Adapter implements the adapter.Adapter interface for Claude Code.
type Adapter struct {
	targetDir string // Override for testing
}

// New creates a new Claude adapter.
// If targetDir is empty, defaults to ~/.claude
func New(targetDir string) *Adapter {
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			targetDir = ".claude" // Fallback
		} else {
			targetDir = filepath.Join(home, ".claude")
		}
	}
	return &Adapter{targetDir: targetDir}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "claude"
}

// AssetMapping returns how tai asset types map to Claude paths.
func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypePersona:  "output-styles",
		config.AssetTypeDelegate: "agents",
		config.AssetTypeSkill:    "skills",
		// Note: "file" type uses asset.Dest directly, not this mapping
	}
}

// TargetDir returns the Claude config directory.
func (a *Adapter) TargetDir() string {
	return a.targetDir
}

// MapAssetPath maps a tai asset to a Claude target path.
func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	return adapter.DefaultMapAssetPath(a.AssetMapping(), asset)
}

// TransformContent transforms asset content into Claude's expected format.
// Personas get name + description frontmatter.
// Delegates and skills get name + description + tools frontmatter.
// Files are returned as-is (no frontmatter processing).
func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	switch asset.Type {
	case config.AssetTypePersona:
		return a.transformPersona(asset), nil
	case config.AssetTypeDelegate, config.AssetTypeSkill:
		return a.transformAgent(asset), nil
	case config.AssetTypeFile:
		// Files are copied as-is, no transformation
		return asset.Content, nil
	default:
		// Unknown types: return content as-is
		return asset.Content, nil
	}
}

// transformPersona renders Claude output-style frontmatter (name, description).
func (a *Adapter) transformPersona(asset *config.ResolvedAsset) []byte {
	fields := map[string]string{fm.FieldName: asset.Metadata[fm.FieldName]}
	order := []string{fm.FieldName}

	if desc := asset.Metadata[fm.FieldDescription]; desc != "" {
		fields[fm.FieldDescription] = desc
		order = append(order, fm.FieldDescription)
	}

	return fm.Render(fields, order, asset.Content)
}

// transformAgent renders Claude agent/skill frontmatter (name, description, tools).
func (a *Adapter) transformAgent(asset *config.ResolvedAsset) []byte {
	fields := map[string]string{fm.FieldName: asset.Metadata[fm.FieldName]}
	order := []string{fm.FieldName}

	if desc := asset.Metadata[fm.FieldDescription]; desc != "" {
		fields[fm.FieldDescription] = desc
		order = append(order, fm.FieldDescription)
	}

	if tools := asset.Metadata[fm.FieldTools]; tools != "" {
		fields[fm.FieldTools] = tools
		order = append(order, fm.FieldTools)
	}

	if model := asset.Metadata[fm.FieldModel]; model != "" {
		fields[fm.FieldModel] = model
		order = append(order, fm.FieldModel)
	}

	return fm.Render(fields, order, asset.Content)
}

// Run invokes claude with the given arguments.
func (a *Adapter) Run(args []string) error {
	return adapter.RunBinary("claude", args)
}

// SupportedConfigAssetTypes returns the config asset types Claude supports.
func (a *Adapter) SupportedConfigAssetTypes() []string {
	return []string{config.AssetTypeStatusLine}
}

// InstallWarnings returns Claude-specific advisory install warnings.
func (a *Adapter) InstallWarnings(_ *config.Asset) []string {
	return nil
}

// SettingsFilePath returns the path to Claude's settings file.
func (a *Adapter) SettingsFilePath(scope string) string {
	if scope == "project" {
		// Project-scoped settings would be in .claude/settings.json
		// but we need the working directory context for that
		return ".claude/settings.json"
	}
	// User-scoped settings
	return filepath.Join(a.targetDir, "settings.json")
}

// TransformStatusLine transforms a tai status line config to Claude's format.
func (a *Adapter) TransformStatusLine(cfg *config.StatusLineConfig, rawNative []byte) (map[string]any, error) {
	// If native format, parse and return as-is
	if rawNative != nil {
		var native map[string]any
		if err := json.Unmarshal(rawNative, &native); err != nil {
			// Try YAML if JSON fails
			return nil, fmt.Errorf("native status line config is not valid JSON or YAML: %w", err)
		}
		return native, nil
	}

	// Transform tai format to Claude format
	result := make(map[string]any)

	switch cfg.Level {
	case config.StatusLineLevelCustom:
		result["type"] = "command"
		result["command"] = cfg.Command
		if cfg.Padding != nil {
			result["padding"] = *cfg.Padding
		}
	case config.StatusLineLevelHidden:
		result["style"] = "hidden"
	case config.StatusLineLevelMinimal:
		result["style"] = "minimal"
		a.applyShowElements(result, cfg.Show)
	case config.StatusLineLevelStandard:
		result["style"] = "standard"
		a.applyShowElements(result, cfg.Show)
	case config.StatusLineLevelDetailed:
		result["style"] = "detailed"
		a.applyShowElements(result, cfg.Show)
	default:
		return nil, fmt.Errorf("unknown status line level: %s", cfg.Level)
	}

	return result, nil
}

// SettingsKey returns the JSON key name for a config asset type in Claude's settings.json.
func (a *Adapter) SettingsKey(assetType string) string {
	switch assetType {
	case config.AssetTypeStatusLine:
		return StatusLineSettingsKey
	default:
		return ""
	}
}

// MainFileName returns the main file name for a directory asset type.
func (a *Adapter) MainFileName(assetType string) string {
	return config.MainFileName(assetType)
}

// UnsupportedFileAssetTypes returns nil - Claude supports all file asset types.
func (a *Adapter) UnsupportedFileAssetTypes() []string {
	return nil
}

// ProvideVariables returns Claude-specific template variables.
func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         "claude",
		"target_name_display": "Claude",
		"target_agents_file":  "CLAUDE.md",
	}
}

// applyShowElements sets Claude's show* flags based on tai's show elements.
func (a *Adapter) applyShowElements(result map[string]any, show []string) {
	// Create a set of requested elements
	showSet := make(map[string]bool)
	for _, elem := range show {
		showSet[elem] = true
	}

	// Map tai show elements to Claude's boolean flags
	if len(show) > 0 {
		// Only set flags if show is explicitly specified
		result["showCost"] = showSet["cost"]
		result["showModel"] = showSet["model"]
		result["showTokens"] = showSet["tokens"]
		result["showLatency"] = showSet["latency"]
		// Note: "context" maps to nothing in current Claude Code; reserved for future use
	}
}
