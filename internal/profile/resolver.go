// Package profile handles profile resolution.
package profile

import (
	"fmt"
	"time"

	"github.com/amterp/tai/internal/bundle"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/dependency"
	"github.com/amterp/tai/internal/source"
)

// Logger is an interface for logging messages.
type Logger interface {
	Printf(format string, args ...any)
}

// noopLogger discards all log messages.
type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}

// ResolvedProfile contains the fully resolved assets for a profile.
type ResolvedProfile struct {
	Name               string
	Target             string
	Options            map[string]any
	Vars               map[string]string // Template variables from profile definition
	Assets             []*config.Asset
	PostInstall        []string // Hook run after a successful install
	PostInstallTimeout time.Duration
	Scope              string // Default installation scope for config assets
}

// Resolver handles profile resolution.
type Resolver struct {
	profiles       map[string]*config.Profile
	bundles        map[string]*config.Bundle
	sources        map[string]*source.LoadedSource
	bundleResolver *bundle.Resolver
	depResolver    *dependency.Resolver
	logger         Logger
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
		depResolver:    dependency.NewResolver(sources),
		logger:         noopLogger{}, // Discard by default
	}
}

// SetLogger sets the logger for override messages.
func (r *Resolver) SetLogger(logger Logger) {
	r.logger = logger
	r.depResolver.SetLogger(logger)
	r.bundleResolver.SetLogger(logger)
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
		key := assetDedupeKey(asset)
		if _, exists := assetMap[key]; !exists {
			order = append(order, key)
		}
		assetMap[key] = asset
	}

	// Resolve and add direct assets (override bundle assets)
	for _, entry := range profile.Assets {
		asset, err := r.resolveAssetRef(entry)
		if err != nil {
			return nil, fmt.Errorf("profile %q: %w", profile.Name, err)
		}

		key := assetDedupeKey(asset)
		if existing, exists := assetMap[key]; exists {
			r.logger.Printf("Override: %s (bundle -> profile direct)", existing.Type+"/"+existing.Name)
		}
		if _, exists := assetMap[key]; !exists {
			order = append(order, key)
		}
		assetMap[key] = asset
	}

	// Build asset list from order
	assets := make([]*config.Asset, 0, len(order))
	for _, key := range order {
		assets = append(assets, assetMap[key])
	}

	// Resolve asset dependencies (auto-include depends)
	assets, err = r.depResolver.Resolve(assets)
	if err != nil {
		return nil, fmt.Errorf("resolving dependencies for profile %q: %w", profile.Name, err)
	}

	return &ResolvedProfile{
		Name:               profile.Name,
		Target:             profile.Target,
		Options:            profile.Options,
		Vars:               profile.Vars,
		Assets:             assets,
		PostInstall:        profile.PostInstall,
		PostInstallTimeout: profile.ResolvedPostInstallTimeout(),
		Scope:              profile.ResolvedScope(),
	}, nil
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
	// Propagate the per-asset scope override (config types); empty inherits
	// the profile's scope.
	asset.Scope = entry.Scope

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

// assetDedupeKey returns a key for deduplicating assets.
// For file types, the key includes dest since the same file can be installed
// to multiple destinations. For other types, it's just type/name.
func assetDedupeKey(asset *config.Asset) string {
	if asset.Type == config.AssetTypeFile {
		return asset.Type + "/" + asset.Name + "@" + asset.Dest
	}
	return asset.Type + "/" + asset.Name
}
