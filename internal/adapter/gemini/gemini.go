// Package gemini provides the adapter for Gemini CLI.
package gemini

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	fm "github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/template"
)

func init() {
	adapter.DefaultRegistry.Register(New(""))
}

// Adapter implements the adapter.Adapter interface for Gemini CLI.
type Adapter struct {
	targetDir string // Override for testing
}

// New creates a new Gemini adapter.
// If targetDir is empty, defaults to $GEMINI_HOME or ~/.gemini.
func New(targetDir string) *Adapter {
	if targetDir == "" {
		targetDir = os.Getenv("GEMINI_HOME")
	}
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			targetDir = ".gemini" // Fallback
		} else {
			targetDir = filepath.Join(home, ".gemini")
		}
	}
	return &Adapter{targetDir: targetDir}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "gemini"
}

// AssetMapping returns how tai asset types map to Gemini paths.
// Gemini CLI has no native persona concept, so personas are mapped to skills.
// Delegates are unsupported and handled via UnsupportedFileAssetTypes.
func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypePersona: "skills",
		config.AssetTypeSkill:   "skills",
		// Note: delegates are in UnsupportedFileAssetTypes, not mapped here
		// Note: "file" type uses asset.Dest directly, not this mapping
	}
}

// TargetDir returns the Gemini config directory.
func (a *Adapter) TargetDir() string {
	return a.targetDir
}

// MapAssetPath maps a tai asset to a Gemini target path.
func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	return adapter.DefaultMapAssetPath(a.AssetMapping(), asset)
}

// TransformContent transforms asset content into Gemini's expected format.
// Personas are mapped to skills, so they get the same frontmatter (name + description).
// Skills get name + description frontmatter.
// Files are returned as-is.
func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	switch asset.Type {
	case config.AssetTypePersona, config.AssetTypeSkill:
		return a.transformSkill(asset), nil
	case config.AssetTypeFile:
		return asset.Content, nil
	default:
		return asset.Content, nil
	}
}

// transformSkill renders Gemini skill frontmatter (name + description).
// Used for both skills and personas (which are mapped to skills).
func (a *Adapter) transformSkill(asset *config.ResolvedAsset) []byte {
	fields := map[string]string{fm.FieldName: asset.Metadata[fm.FieldName]}
	order := []string{fm.FieldName}

	if desc := asset.Metadata[fm.FieldDescription]; desc != "" {
		fields[fm.FieldDescription] = desc
		order = append(order, fm.FieldDescription)
	}

	return fm.Render(fields, order, asset.Content)
}

// Run invokes gemini with the given arguments.
func (a *Adapter) Run(args []string) error {
	return adapter.RunBinary("gemini", args)
}

// SupportedConfigAssetTypes returns the config asset types Gemini supports.
// No config assets are supported in the initial implementation.
func (a *Adapter) SupportedConfigAssetTypes() []string {
	return []string{}
}

// UnsupportedFileAssetTypes returns file asset types Gemini doesn't support.
// Delegates are skipped because Gemini CLI has no user-defined sub-agent concept.
func (a *Adapter) UnsupportedFileAssetTypes() []string {
	return []string{config.AssetTypeDelegate}
}

// InstallWarnings returns Gemini-specific advisory install warnings.
func (a *Adapter) InstallWarnings(asset *config.Asset) []string {
	switch asset.Type {
	case config.AssetTypePersona:
		return []string{
			fmt.Sprintf("Gemini CLI does not support personas - installed as skill instead: skills/%s", asset.Name),
		}
	default:
		return nil
	}
}

// SettingsFilePath returns the settings file path for Gemini.
// Returns empty - no config assets supported yet.
func (a *Adapter) SettingsFilePath(string) string {
	return ""
}

// TransformStatusLine is unsupported for Gemini CLI.
func (a *Adapter) TransformStatusLine(*config.StatusLineConfig, []byte) (map[string]any, error) {
	return nil, fmt.Errorf("statusline is not supported by target gemini")
}

// SettingsKey returns the JSON key name for a config asset type.
func (a *Adapter) SettingsKey(string) string {
	return ""
}

// MainFileName returns the main file name for a directory asset type.
func (a *Adapter) MainFileName(assetType string) string {
	return config.MainFileName(assetType)
}

// ProvideVariables returns Gemini-specific template variables.
func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         "gemini",
		"target_name_display": "Gemini",
		"target_agents_file":  "GEMINI.md",
	}
}
