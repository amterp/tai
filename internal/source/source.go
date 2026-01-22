// Package source handles source loading and asset discovery.
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/config"
)

// LoadedSource represents a source with its resolved path and discovered assets.
type LoadedSource struct {
	config.Source
	ResolvedPath string              // Absolute filesystem path
	Assets       map[string][]string // asset type -> list of asset names
}

// Loader handles loading and resolving sources.
type Loader struct {
	configRepoPath string // Path to the tai config repo (for relative path resolution)
}

// NewLoader creates a new source loader.
func NewLoader(configRepoPath string) *Loader {
	return &Loader{configRepoPath: configRepoPath}
}

// LoadAll loads and resolves all sources from a tai config.
func (l *Loader) LoadAll(sources []config.Source) (map[string]*LoadedSource, error) {
	loaded := make(map[string]*LoadedSource)

	for _, src := range sources {
		ls, err := l.Load(src)
		if err != nil {
			return nil, fmt.Errorf("loading source %q: %w", src.Name, err)
		}
		loaded[src.Name] = ls
	}

	return loaded, nil
}

// Load loads a single source.
func (l *Loader) Load(src config.Source) (*LoadedSource, error) {
	resolvedPath, err := l.resolvePath(src)
	if err != nil {
		return nil, err
	}

	// Verify path exists
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("source path does not exist: %s", resolvedPath)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source path is not a directory: %s", resolvedPath)
	}

	ls := &LoadedSource{
		Source:       src,
		ResolvedPath: resolvedPath,
		Assets:       make(map[string][]string),
	}

	// Discover assets
	if err := l.discoverAssets(ls); err != nil {
		return nil, fmt.Errorf("discovering assets: %w", err)
	}

	return ls, nil
}

// resolvePath resolves the source path to an absolute filesystem path.
func (l *Loader) resolvePath(src config.Source) (string, error) {
	path := src.Path

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Handle relative paths
	if !filepath.IsAbs(path) {
		path = filepath.Join(l.configRepoPath, path)
	}

	// For tai-repo sources, point to assets/ subdirectory
	if src.Type == "tai-repo" {
		path = filepath.Join(path, "assets")
	}

	return filepath.Clean(path), nil
}

// discoverAssets discovers all assets in a loaded source.
func (l *Loader) discoverAssets(ls *LoadedSource) error {
	for _, assetType := range config.ValidAssetTypes() {
		assetDir := filepath.Join(ls.ResolvedPath, assetType)
		assets, err := l.discoverAssetsInDir(assetDir, assetType)
		if err != nil {
			return err
		}
		ls.Assets[assetType] = assets
	}
	return nil
}

// discoverAssetsInDir discovers assets in a specific asset type directory.
func (l *Loader) discoverAssetsInDir(dir, assetType string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // Directory doesn't exist - no assets of this type
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s directory: %w", assetType, err)
	}

	var assets []string
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files
		if strings.HasPrefix(name, ".") {
			continue
		}

		// Skills are directories, everything else are files
		if assetType == config.AssetTypeSkills {
			if entry.IsDir() {
				assets = append(assets, name)
			}
		} else {
			if !entry.IsDir() {
				assets = append(assets, name)
			}
		}
	}

	return assets, nil
}

// GetAsset returns the full path to an asset.
// Returns false if the path escapes the source directory (path traversal protection).
func (ls *LoadedSource) GetAsset(assetPath string) (string, bool) {
	// assetPath is like "personas/senior-swe.md" or "skills/my-skill"
	fullPath := filepath.Join(ls.ResolvedPath, assetPath)

	// Clean both paths and verify the result is still within the source directory
	cleanedFull := filepath.Clean(fullPath)
	cleanedBase := filepath.Clean(ls.ResolvedPath)
	if !strings.HasPrefix(cleanedFull, cleanedBase+string(filepath.Separator)) &&
		cleanedFull != cleanedBase {
		return "", false // Path traversal attempt
	}

	_, err := os.Stat(fullPath)
	if err != nil {
		return "", false
	}
	return fullPath, true
}

// HasAsset checks if the source has the given asset.
func (ls *LoadedSource) HasAsset(assetPath string) bool {
	_, ok := ls.GetAsset(assetPath)
	return ok
}

// ResolveAsset resolves an asset path to a config.Asset.
func (ls *LoadedSource) ResolveAsset(assetPath string) (*config.Asset, error) {
	fullPath, ok := ls.GetAsset(assetPath)
	if !ok {
		return nil, fmt.Errorf("asset not found: %s", assetPath)
	}

	// Parse the asset path
	parts := strings.SplitN(assetPath, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid asset path format: %s (expected type/name)", assetPath)
	}

	assetType := parts[0]
	assetName := parts[1]

	if !config.IsValidAssetType(assetType) {
		return nil, fmt.Errorf("invalid asset type: %s", assetType)
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, err
	}

	return &config.Asset{
		Type:       assetType,
		Name:       assetName,
		SourceName: ls.Name,
		SourcePath: fullPath,
		IsDir:      info.IsDir(),
	}, nil
}

// ListAllAssets returns all assets in this source as a flat list of paths.
func (ls *LoadedSource) ListAllAssets() []string {
	var all []string
	for assetType, names := range ls.Assets {
		for _, name := range names {
			all = append(all, filepath.Join(assetType, name))
		}
	}
	return all
}
