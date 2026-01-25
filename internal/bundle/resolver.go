// Package bundle handles bundle loading and resolution.
package bundle

import (
	"fmt"
	"strings"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
)

// ResolvedBundle contains the fully resolved assets for a bundle.
type ResolvedBundle struct {
	Name   string
	Assets []*config.Asset
}

// Resolver handles bundle resolution.
type Resolver struct {
	bundles map[string]*config.Bundle
	sources map[string]*source.LoadedSource
}

// NewResolver creates a new bundle resolver.
func NewResolver(bundles map[string]*config.Bundle, sources map[string]*source.LoadedSource) *Resolver {
	return &Resolver{
		bundles: bundles,
		sources: sources,
	}
}

// Resolve resolves a bundle by name, following includes and resolving assets.
func (r *Resolver) Resolve(name string) (*ResolvedBundle, error) {
	return r.resolveWithPath(name, nil)
}

// resolveWithPath resolves a bundle while tracking the resolution path for cycle detection.
func (r *Resolver) resolveWithPath(name string, path []string) (*ResolvedBundle, error) {
	// Check for cycles
	for _, p := range path {
		if p == name {
			cycle := append(path, name)
			return nil, fmt.Errorf("circular bundle reference detected: %s", strings.Join(cycle, " -> "))
		}
	}

	bundle, ok := r.bundles[name]
	if !ok {
		return nil, fmt.Errorf("bundle not found: %s", name)
	}

	// Track current path
	currentPath := append(path, name)

	// Collect assets with deduplication
	assetMap := make(map[string]*config.Asset) // assetPath -> asset (later wins)
	order := []string{}                         // Track insertion order

	// First, resolve included bundles
	for _, includedName := range bundle.Bundles {
		included, err := r.resolveWithPath(includedName, currentPath)
		if err != nil {
			return nil, fmt.Errorf("resolving included bundle %q: %w", includedName, err)
		}

		for _, asset := range included.Assets {
			assetKey := assetDedupeKey(asset)
			if _, exists := assetMap[assetKey]; !exists {
				order = append(order, assetKey)
			}
			assetMap[assetKey] = asset
		}
	}

	// Then, resolve direct assets (these override included bundle assets)
	for _, entry := range bundle.Assets {
		asset, err := r.resolveAssetRef(entry)
		if err != nil {
			return nil, fmt.Errorf("bundle %q: %w", name, err)
		}

		assetKey := assetDedupeKey(asset)
		if _, exists := assetMap[assetKey]; !exists {
			order = append(order, assetKey)
		}
		assetMap[assetKey] = asset
	}

	// Build result in order
	result := &ResolvedBundle{
		Name:   name,
		Assets: make([]*config.Asset, 0, len(order)),
	}
	for _, key := range order {
		result.Assets = append(result.Assets, assetMap[key])
	}

	return result, nil
}

// resolveAssetRef resolves a structured asset entry to an Asset.
func (r *Resolver) resolveAssetRef(entry config.AssetEntry) (*config.Asset, error) {
	sourceName, assetPath := config.ParseAssetReference(entry.Path)

	src, ok := r.sources[sourceName]
	if !ok {
		return nil, fmt.Errorf("source not found: %s (in asset ref %q)", sourceName, entry.Path)
	}

	asset, err := src.ResolveAsset(assetPath, entry.Type)
	if err != nil {
		return nil, fmt.Errorf("resolving asset %q from source %q: %w", assetPath, sourceName, err)
	}

	// Propagate Dest from entry to asset (used by file type)
	asset.Dest = entry.Dest

	return asset, nil
}

// ResolveMultiple resolves multiple bundles in order, merging their assets.
// Later bundles override earlier ones on conflict (deduplication).
func (r *Resolver) ResolveMultiple(names []string) (*ResolvedBundle, []Override, error) {
	assetMap := make(map[string]*config.Asset)
	bundleMap := make(map[string]string) // assetKey -> bundle name that provided it
	order := []string{}
	var overrides []Override

	for _, name := range names {
		resolved, err := r.Resolve(name)
		if err != nil {
			return nil, nil, err
		}

		for _, asset := range resolved.Assets {
			assetKey := assetDedupeKey(asset)
			if existing, exists := assetMap[assetKey]; exists {
				// Record the override with correct bundle names
				overrides = append(overrides, Override{
					AssetPath:  assetKey,
					FromBundle: bundleMap[assetKey],
					ToBundle:   name,
					FromSource: existing.SourceName,
					ToSource:   asset.SourceName,
				})
			} else {
				order = append(order, assetKey)
			}
			assetMap[assetKey] = asset
			bundleMap[assetKey] = name
		}
	}

	result := &ResolvedBundle{
		Name:   strings.Join(names, "+"),
		Assets: make([]*config.Asset, 0, len(order)),
	}
	for _, key := range order {
		result.Assets = append(result.Assets, assetMap[key])
	}

	return result, overrides, nil
}

// Override records when an asset from one bundle overrides another.
type Override struct {
	AssetPath  string
	FromBundle string
	ToBundle   string
	FromSource string
	ToSource   string
}

// assetDedupeKey returns a key for deduplicating assets.
// For file types, the key includes dest since the same file can be installed
// to multiple destinations. For other types, it's just type/name.
func assetDedupeKey(asset *config.Asset) string {
	if asset.Type == config.AssetTypeFile {
		return asset.Type + "/" + asset.Name + "@" + asset.Dest
	}
	return asset.Type + "/" + asset.Name
}
