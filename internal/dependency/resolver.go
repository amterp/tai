// Package dependency handles resolving asset dependencies declared in frontmatter.
package dependency

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/source"
)

// Logger is an interface for logging messages.
type Logger interface {
	Printf(format string, args ...any)
}

// noopLogger discards all log messages.
type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}

// Resolver resolves asset dependencies declared in frontmatter.
type Resolver struct {
	sources map[string]*source.LoadedSource
	logger  Logger
}

// NewResolver creates a new dependency resolver.
func NewResolver(sources map[string]*source.LoadedSource) *Resolver {
	return &Resolver{
		sources: sources,
		logger:  noopLogger{},
	}
}

// SetLogger sets the logger for dependency resolution messages.
func (r *Resolver) SetLogger(logger Logger) {
	r.logger = logger
}

// Resolve expands the asset list to include all dependencies.
// It reads each asset's frontmatter, extracts dependencies, and recursively
// resolves them. Cycle detection prevents infinite loops.
//
// Dependencies are prepended to the list (depth-first), so they appear before
// the assets that depend on them. Explicit bundle/profile assets override
// auto-included dependencies (later wins).
func (r *Resolver) Resolve(assets []*config.Asset) ([]*config.Asset, error) {
	// Track which assets are explicitly included (bundle/profile direct assets)
	explicitAssets := make(map[string]bool)
	for _, asset := range assets {
		key := assetKey(asset)
		explicitAssets[key] = true
	}

	// Resolve dependencies for each asset
	var allDeps []*config.Asset
	seen := make(map[string]bool) // Track all assets we've seen (explicit + deps)

	// Mark explicit assets as seen
	for _, asset := range assets {
		seen[assetKey(asset)] = true
	}

	for _, asset := range assets {
		deps, err := r.resolveDeps(asset, nil, seen, explicitAssets)
		if err != nil {
			return nil, err
		}
		allDeps = append(allDeps, deps...)
	}

	// Build final list: dependencies first (in order discovered), then explicit assets
	// This ensures dependencies are available before the assets that need them
	result := make([]*config.Asset, 0, len(allDeps)+len(assets))

	// Deduplicate while preserving order
	added := make(map[string]bool)
	for _, dep := range allDeps {
		key := assetKey(dep)
		if !added[key] {
			result = append(result, dep)
			added[key] = true
		}
	}

	// Add explicit assets (these override any conflicting deps due to later-wins rule)
	for _, asset := range assets {
		key := assetKey(asset)
		if added[key] {
			// Dependency was already added, but explicit asset overrides
			r.logger.Printf("Dependency override: %s (auto-included -> explicit)", key)
		}
		// Find and replace, or append
		found := false
		for i, existing := range result {
			if assetKey(existing) == key {
				result[i] = asset
				found = true
				break
			}
		}
		if !found {
			result = append(result, asset)
		}
		added[key] = true
	}

	return result, nil
}

// resolveDeps recursively resolves dependencies for an asset.
// path tracks the resolution path for cycle detection.
// seen tracks all assets we've already processed.
// explicitAssets tracks assets explicitly included in bundle/profile.
func (r *Resolver) resolveDeps(
	asset *config.Asset,
	path []string,
	seen map[string]bool,
	explicitAssets map[string]bool,
) ([]*config.Asset, error) {
	// Get dependencies from asset frontmatter
	deps, err := r.extractDeps(asset)
	if err != nil {
		return nil, fmt.Errorf("extracting dependencies from %s/%s: %w",
			asset.Type, asset.Name, err)
	}

	if len(deps) == 0 {
		return nil, nil
	}

	currentKey := assetKey(asset)
	currentPath := append(path, currentKey)

	var result []*config.Asset

	for _, dep := range deps {
		// Resolve the dependency to an asset
		depAsset, err := r.resolveDepRef(dep, asset.SourceName)
		if err != nil {
			return nil, fmt.Errorf("resolving dependency %q in %s/%s: %w",
				dep.Path, asset.Type, asset.Name, err)
		}

		depKey := assetKey(depAsset)

		// Check for cycles
		for i, p := range currentPath {
			if p == depKey {
				cycle := append(currentPath[i:], depKey)
				return nil, fmt.Errorf("circular dependency detected: %s",
					strings.Join(cycle, " -> "))
			}
		}

		// Skip if already seen (either explicit or already resolved as dep)
		if seen[depKey] {
			if explicitAssets[depKey] {
				r.logger.Printf("Dependency %s already explicitly included, skipping auto-include",
					depKey)
			}
			continue
		}

		seen[depKey] = true

		// Recursively resolve this dependency's dependencies (depth-first)
		transitiveDeps, err := r.resolveDeps(depAsset, currentPath, seen, explicitAssets)
		if err != nil {
			return nil, err
		}

		// Add transitive deps first (depth-first order)
		result = append(result, transitiveDeps...)

		// Then add this dependency
		r.logger.Printf("Auto-including dependency: %s (required by %s/%s)",
			depKey, asset.Type, asset.Name)
		result = append(result, depAsset)
	}

	return result, nil
}

// extractDeps reads an asset's content and extracts dependencies from frontmatter.
func (r *Resolver) extractDeps(asset *config.Asset) ([]frontmatter.DependencyEntry, error) {
	// For directory assets, read the main file
	var contentPath string
	if asset.IsDir {
		mainFile := config.MainFileName(asset.Type)
		if mainFile == "" {
			return nil, nil // Directory type without main file
		}
		contentPath = filepath.Join(asset.SourcePath, mainFile)
	} else {
		contentPath = asset.SourcePath
	}

	// File assets don't have frontmatter processing
	if asset.Type == config.AssetTypeFile {
		return nil, nil
	}

	content, err := os.ReadFile(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No main file = no deps
		}
		return nil, err
	}

	parsed, err := frontmatter.Parse(content)
	if err != nil {
		return nil, err
	}

	return parsed.Depends, nil
}

// resolveDepRef resolves a dependency reference to an asset.
// The dep.Path format is "type-dir/name" or "source:type-dir/name".
// Examples: "delegates/bug-hunter.md", "other-source:skills/my-skill"
func (r *Resolver) resolveDepRef(dep frontmatter.DependencyEntry, defaultSource string) (*config.Asset, error) {
	// Parse source and asset path
	sourceName, assetPath := config.ParseAssetReference(dep.Path)

	// Extract type from the first path segment (e.g., "delegates" -> "delegate")
	parts := strings.SplitN(assetPath, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid dependency path %q: must be type-dir/name (e.g., delegates/bug-hunter.md)", dep.Path)
	}

	typeDir := parts[0]
	assetType := config.AssetTypeFromDirName(typeDir)
	if !config.IsValidAssetType(assetType) {
		return nil, fmt.Errorf("invalid dependency path %q: unknown type directory %q", dep.Path, typeDir)
	}

	// If no source specified and we have a default, check local first then fall back
	if sourceName == "local" && defaultSource != "" && defaultSource != "local" {
		if src, ok := r.sources["local"]; ok {
			if src.HasAsset(assetPath) {
				sourceName = "local"
			} else {
				sourceName = defaultSource
			}
		} else {
			sourceName = defaultSource
		}
	}

	src, ok := r.sources[sourceName]
	if !ok {
		return nil, fmt.Errorf("source not found: %s", sourceName)
	}

	return src.ResolveAsset(assetPath, assetType)
}

// assetKey returns a unique key for an asset (type/name).
func assetKey(asset *config.Asset) string {
	if asset.Type == config.AssetTypeFile {
		return asset.Type + "/" + asset.Name + "@" + asset.Dest
	}
	return asset.Type + "/" + asset.Name
}
