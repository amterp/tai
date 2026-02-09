// Package codex provides the adapter for Codex.
package codex

import (
	"fmt"
	"os"
	"os/exec"
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
func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypePersona:  "personas",
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
func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	// File type uses Dest directly
	if asset.Type == config.AssetTypeFile {
		if asset.Dest == "" {
			// Root level with original name
			return asset.Name
		}
		// Interpret dest based on its form:
		// - Ends with "/" -> explicit directory, use dest/name
		// - Basename contains "." -> full path (dest IS the final path)
		// - Otherwise -> directory, use dest/name
		if strings.HasSuffix(asset.Dest, "/") {
			return filepath.Join(strings.TrimSuffix(asset.Dest, "/"), asset.Name)
		}
		basename := filepath.Base(asset.Dest)
		if strings.Contains(basename, ".") {
			return asset.Dest
		}
		return filepath.Join(asset.Dest, asset.Name)
	}

	mapping := a.AssetMapping()
	targetType, ok := mapping[asset.Type]
	if !ok {
		// Unknown type - use as-is
		return filepath.Join(asset.Type, asset.Name)
	}

	// Both file and directory assets use <target-type>/<asset-name>
	return filepath.Join(targetType, asset.Name)
}

// TransformContent transforms asset content into Codex-compatible format.
// Files are returned as-is. Other types get frontmatter re-rendered from tai metadata.
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

func (a *Adapter) transformPersona(asset *config.ResolvedAsset) []byte {
	fields := map[string]string{fm.FieldName: asset.Metadata[fm.FieldName]}
	order := []string{fm.FieldName}

	if desc := asset.Metadata[fm.FieldDescription]; desc != "" {
		fields[fm.FieldDescription] = desc
		order = append(order, fm.FieldDescription)
	}

	return fm.Render(fields, order, asset.Content)
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
	cmd := exec.Command("codex", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
			fmt.Sprintf("Codex does not natively consume persona assets - installed as passive reference file: personas/%s", asset.Name),
		}
	case config.AssetTypeDelegate:
		return []string{
			fmt.Sprintf("Codex does not natively consume delegate assets - installed as passive reference file: delegates/%s", asset.Name),
		}
	default:
		return nil
	}
}

// SettingsFilePath returns the settings file path for Codex.
func (a *Adapter) SettingsFilePath(string) string {
	return ""
}

// TransformStatusLine is unsupported for Codex.
func (a *Adapter) TransformStatusLine(*config.StatusLineConfig, []byte) (map[string]any, error) {
	return nil, fmt.Errorf("statusline is not supported by target codex")
}

// SettingsKey returns the JSON key name for a config asset type.
func (a *Adapter) SettingsKey(string) string {
	return ""
}

// MainFileName returns the main file name for a directory asset type.
func (a *Adapter) MainFileName(assetType string) string {
	return config.MainFileName(assetType)
}

// ProvideVariables returns Codex-specific template variables.
func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         "codex",
		"target_name_display": "Codex",
		"target_agents_file":  "AGENTS.md",
	}
}
