// Package profile handles profile resolution.
package profile

import (
	"fmt"
	"io"
	"log"

	"github.com/amterp/tai/internal/bundle"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
)

// ResolvedProfile contains the fully resolved assets for a profile.
type ResolvedProfile struct {
	Name    string
	Target  string
	Options map[string]any
	Assets  []*config.Asset
}

// Resolver handles profile resolution.
type Resolver struct {
	profiles       map[string]*config.Profile
	bundles        map[string]*config.Bundle
	sources        map[string]*source.LoadedSource
	bundleResolver *bundle.Resolver
	logger         *log.Logger
}

// NewResolver creates a new profile resolver.
func NewResolver(
	profiles map[string]*config.Profile,
	bundles map[string]*config.Bundle,
	sources map[string]*source.LoadedSource,
) *Resolver {
	return &Resolver{
		profiles:       profiles,
		bundles:        bundles,
		sources:        sources,
		bundleResolver: bundle.NewResolver(bundles, sources),
		logger:         log.New(io.Discard, "", 0), // Discard by default
	}
}

// SetLogger sets the logger for override messages.
func (r *Resolver) SetLogger(logger *log.Logger) {
	r.logger = logger
}

// Resolve resolves a profile by name.
func (r *Resolver) Resolve(name string) (*ResolvedProfile, error) {
	profile, ok := r.profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile not found: %s", name)
	}

	return r.resolveProfile(profile)
}

// resolveProfile resolves a profile.
func (r *Resolver) resolveProfile(profile *config.Profile) (*ResolvedProfile, error) {
	// Resolve all bundles in order
	resolved, overrides, err := r.bundleResolver.ResolveMultiple(profile.Bundles)
	if err != nil {
		return nil, fmt.Errorf("resolving bundles for profile %q: %w", profile.Name, err)
	}

	// Log overrides
	for _, o := range overrides {
		r.logger.Printf("Override: %s (%s -> %s)", o.AssetPath, o.FromBundle, o.ToBundle)
	}

	// Collect all assets (from bundles)
	assetMap := make(map[string]*config.Asset)
	order := []string{}

	for _, asset := range resolved.Assets {
		key := asset.Type + "/" + asset.Name
		if _, exists := assetMap[key]; !exists {
			order = append(order, key)
		}
		assetMap[key] = asset
	}

	// Resolve and add direct assets (override bundle assets)
	for _, assetRef := range profile.Assets {
		asset, err := r.resolveAssetRef(assetRef)
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", profile.Name, err)
		}

		key := asset.Type + "/" + asset.Name
		if existing, exists := assetMap[key]; exists {
			r.logger.Printf("Override: %s (bundle -> profile direct)", existing.Type+"/"+existing.Name)
		}
		if _, exists := assetMap[key]; !exists {
			order = append(order, key)
		}
		assetMap[key] = asset
	}

	// Build result
	assets := make([]*config.Asset, 0, len(order))
	for _, key := range order {
		assets = append(assets, assetMap[key])
	}

	return &ResolvedProfile{
		Name:    profile.Name,
		Target:  profile.Target,
		Options: profile.Options,
		Assets:  assets,
	}, nil
}

// resolveAssetRef resolves an asset reference string to an Asset.
func (r *Resolver) resolveAssetRef(ref string) (*config.Asset, error) {
	sourceName, assetPath := config.ParseAssetReference(ref)

	src, ok := r.sources[sourceName]
	if !ok {
		return nil, fmt.Errorf("source not found: %s (in asset ref %q)", sourceName, ref)
	}

	asset, err := src.ResolveAsset(assetPath)
	if err != nil {
		return nil, fmt.Errorf("resolving asset %q from source %q: %w", assetPath, sourceName, err)
	}

	return asset, nil
}

// ListProfiles returns all available profile names.
func (r *Resolver) ListProfiles() []string {
	names := make([]string, 0, len(r.profiles))
	for name := range r.profiles {
		names = append(names, name)
	}
	return names
}

// GetProfile returns a profile by name (without resolving).
func (r *Resolver) GetProfile(name string) (*config.Profile, bool) {
	p, ok := r.profiles[name]
	return p, ok
}
