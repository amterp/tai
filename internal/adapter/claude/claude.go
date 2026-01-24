// Package claude provides the adapter for Claude Code.
package claude

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	fm "github.com/amterp/tai/internal/frontmatter"
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
		config.AssetTypePersonas:  "output-styles",
		config.AssetTypeDelegates: "agents",
		config.AssetTypeSkills:    "skills",
		config.AssetTypeDocs:      "", // Root level (CLAUDE.md, etc.)
	}
}

// TargetDir returns the Claude config directory.
func (a *Adapter) TargetDir() string {
	return a.targetDir
}

// MapAssetPath maps a tai asset to a Claude target path.
func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	mapping := a.AssetMapping()
	targetType, ok := mapping[asset.Type]
	if !ok {
		// Unknown type - use as-is
		return filepath.Join(asset.Type, asset.Name)
	}

	if targetType == "" {
		// Docs go to root level
		return asset.Name
	}

	return filepath.Join(targetType, asset.Name)
}

// TransformContent transforms asset content into Claude's expected format.
// Personas get name + description frontmatter.
// Delegates and skills get name + description + tools frontmatter.
// Docs are returned as body only (no target frontmatter).
func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	switch asset.Type {
	case config.AssetTypePersonas:
		return a.transformPersona(asset), nil
	case config.AssetTypeDelegates, config.AssetTypeSkills:
		return a.transformAgent(asset), nil
	default:
		// Docs: body only, no target frontmatter
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

	return fm.Render(fields, order, asset.Content)
}

// Run invokes claude with the given arguments.
func (a *Adapter) Run(args []string) error {
	cmd := exec.Command("claude", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
