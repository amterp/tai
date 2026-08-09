// Package codex provides the adapter for Codex.
package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	fm "github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/template"
)

func init() {
	adapter.DefaultRegistry.Register(New(""))
}

// Adapter implements the adapter.Adapter interface for Codex.
type Adapter struct {
	targetDir string // Override for testing
}

// New creates a new Codex adapter.
// If targetDir is empty, defaults to $CODEX_HOME or ~/.codex.
func New(targetDir string) *Adapter {
	if targetDir == "" {
		targetDir = os.Getenv("CODEX_HOME")
	}
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			targetDir = ".codex" // Fallback
		} else {
			targetDir = filepath.Join(home, ".codex")
		}
	}
	return &Adapter{targetDir: targetDir}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "codex"
}

// AssetMapping returns how tai asset types map to Codex paths.
// Codex has no native persona concept, so personas are mapped to skills.
func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypePersona:  "skills",
		config.AssetTypeDelegate: "delegates",
		config.AssetTypeSkill:    "skills",
		// Note: "file" type uses asset.Dest directly, not this mapping
	}
}

// TargetDir returns the Codex config directory.
func (a *Adapter) TargetDir() string {
	return a.targetDir
}

// MapAssetPath maps a tai asset to a Codex target path.
// Single-file assets that map to skills/ are wrapped in a directory
// (skills/name/SKILL.md) because Codex only discovers directory-format skills.
func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	if !asset.IsDir && a.mapsToSkills(asset.Type) {
		name := strings.TrimSuffix(asset.Name, ".md")
		return filepath.Join("skills", name, "SKILL.md")
	}
	return adapter.DefaultMapAssetPath(a.AssetMapping(), asset)
}

// mapsToSkills returns true if the given asset type maps to the skills/ directory.
func (a *Adapter) mapsToSkills(assetType string) bool {
	return a.AssetMapping()[assetType] == "skills"
}

// TransformContent transforms asset content into Codex-compatible format.
// Personas are mapped to skills, so they get the same frontmatter (name + description).
// Files are returned as-is. Other types get frontmatter re-rendered from tai metadata.
func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	switch asset.Type {
	case config.AssetTypePersona, config.AssetTypeSkill:
		return a.transformSkill(asset), nil
	case config.AssetTypeDelegate:
		return a.transformDelegate(asset), nil
	case config.AssetTypeFile:
		return asset.Content, nil
	default:
		return asset.Content, nil
	}
}

func (a *Adapter) transformDelegate(asset *config.ResolvedAsset) []byte {
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

func (a *Adapter) transformSkill(asset *config.ResolvedAsset) []byte {
	fields := map[string]string{fm.FieldName: asset.Metadata[fm.FieldName]}
	order := []string{fm.FieldName}

	if desc := asset.Metadata[fm.FieldDescription]; desc != "" {
		fields[fm.FieldDescription] = desc
		order = append(order, fm.FieldDescription)
	}

	return fm.Render(fields, order, asset.Content)
}

// Run invokes codex with the given arguments.
func (a *Adapter) Run(args []string) error {
	return adapter.RunBinary("codex", args)
}

// SupportedConfigAssetTypes returns the config asset types Codex supports.
func (a *Adapter) SupportedConfigAssetTypes() []string {
	return []string{}
}

// InstallWarnings returns Codex-specific advisory install warnings.
func (a *Adapter) InstallWarnings(asset *config.Asset) []string {
	switch asset.Type {
	case config.AssetTypePersona:
		return []string{
			fmt.Sprintf("Codex does not support personas - installed as skill instead: skills/%s/SKILL.md",
				strings.TrimSuffix(asset.Name, ".md")),
		}
	case config.AssetTypeDelegate:
		return []string{
			fmt.Sprintf("Codex does not natively consume delegate assets - installed as passive reference file: delegates/%s", asset.Name),
		}
	default:
		return nil
	}
}

// ConfigFilePath returns no config file: Codex supports no config asset types
// yet, so nothing has a home here.
func (a *Adapter) ConfigFilePath(string, string) string {
	return ""
}

// SettingsKey returns the JSON key name for a config asset type.
func (a *Adapter) SettingsKey(string) string {
	return ""
}

// MainFileName returns the main file name for a directory asset type.
func (a *Adapter) MainFileName(assetType string) string {
	return config.MainFileName(assetType)
}

// UnsupportedFileAssetTypes returns nil - Codex installs all file asset types
// (though some are passive reference only, see InstallWarnings).
func (a *Adapter) UnsupportedFileAssetTypes() []string {
	return nil
}

// ProvideVariables returns Codex-specific template variables.
func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         "codex",
		"target_name_display": "Codex",
		"target_agents_file":  "AGENTS.md",
	}
}
