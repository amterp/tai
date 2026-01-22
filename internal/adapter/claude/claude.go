// Package claude provides the adapter for Claude Code.
package claude

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
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

// Run invokes claude with the given arguments.
func (a *Adapter) Run(args []string) error {
	cmd := exec.Command("claude", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
