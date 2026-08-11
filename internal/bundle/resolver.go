// Package bundle handles bundle loading and resolution.
package bundle

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
)

// Logger is an interface for logging messages.
type Logger interface {
	Printf(format string, args ...any)
}

// noopLogger discards all log messages.
type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}

// origin is the repo a bundle was loaded from: a source name, or activeRepo
// for the config repo tai is running out of.
//
// Origin is what makes an imported bundle work. Upstream writes
// `assets/skills/dredge` meaning its own repo, and that has to keep meaning
// its own repo once a consumer imports the bundle - otherwise every import
// breaks on the first unprefixed path. Source prefixes are the exception: they
// resolve in the consumer's source map, the only one loaded, so a bundle that
// reaches into a third source requires the consumer to define that source too.
type origin string

const activeRepo origin = ""

// qualify renders a bundle name as it is written in config, which is also how
// it is keyed for cycle detection: a local `core` and a `work:core` are
// different bundles and must not collide.
func (o origin) qualify(name string) string {
	if o == activeRepo {
		return name
	}
	return string(o) + ":" + name
}

// ResolvedBundle contains the fully resolved assets for a bundle.
type ResolvedBundle struct {
	Name   string
	Assets []*config.Asset
}

// Resolver handles bundle resolution.
type Resolver struct {
	bundles map[string]*config.Bundle
	sources map[string]*source.LoadedSource
	logger  Logger
}

// NewResolver creates a new bundle resolver.
func NewResolver(bundles map[string]*config.Bundle, sources map[string]*source.LoadedSource) *Resolver {
	return &Resolver{
		bundles: bundles,
		sources: sources,
		logger:  noopLogger{}, // Discard by default
	}
}

// SetLogger sets the logger for filtered-asset and exclusion messages.
func (r *Resolver) SetLogger(logger Logger) {
	r.logger = logger
}

// Resolve resolves a bundle by name, following includes and resolving assets.
func (r *Resolver) Resolve(name string) (*ResolvedBundle, error) {
	return r.resolveEntry(config.BundleEntry{Name: name}, activeRepo, nil)
}

// resolveEntry resolves one `bundles:` entry: it resolves the bundle the entry
// names, then applies the entry's own exclusions to the fully resolved result.
//
// from is the origin of the config that wrote this entry, which an unprefixed
// name resolves against. path tracks the resolution path for cycle detection.
func (r *Resolver) resolveEntry(entry config.BundleEntry, from origin, path []string) (*ResolvedBundle, error) {
	sourceName, bundleName, qualified := config.ParseBundleReference(entry.Name)

	target := from
	if qualified {
		target = origin(sourceName)
	}

	resolved, err := r.resolveBundle(bundleName, target, path)
	if err != nil {
		return nil, err
	}

	resolved.Assets = r.applyExclusions(resolved.Assets, entry, resolved.Name)
	return resolved, nil
}

// applyExclusions drops the assets an import carved out.
//
// Exclusions apply to the fully resolved asset set, so they reach through
// nested includes: what the user is expressing is "this must not land on my
// machine", not "this must not appear at one particular nesting level".
func (r *Resolver) applyExclusions(assets []*config.Asset, entry config.BundleEntry, bundleName string) []*config.Asset {
	if len(entry.Exclude) == 0 {
		return assets
	}

	matched := make([]bool, len(entry.Exclude))
	kept := make([]*config.Asset, 0, len(assets))
	for _, asset := range assets {
		excluded := false
		for i, pattern := range entry.Exclude {
			if config.MatchesExcludePattern(pattern, asset.RelPath) {
				matched[i] = true
				excluded = true
			}
		}
		if excluded {
			r.logger.Printf("Excluded: %s/%s from bundle %s", asset.Type, asset.Name, bundleName)
			continue
		}
		kept = append(kept, asset)
	}

	// A pattern matching nothing is usually a typo or an upstream rename, but
	// it can also mean upstream simply deleted the asset - which satisfies the
	// intent. Warn rather than fail: breaking a consumer's install over someone
	// else's deletion costs more than the stale line does.
	for i, pattern := range entry.Exclude {
		if !matched[i] {
			r.logger.Printf("Warning: exclude pattern %q in bundle %s matched no assets", pattern, bundleName)
		}
	}

	return kept
}

// resolveBundle resolves a single bundle within a given origin.
func (r *Resolver) resolveBundle(name string, in origin, path []string) (*ResolvedBundle, error) {
	key := in.qualify(name)

	// Check for cycles
	for _, p := range path {
		if p == key {
			cycle := append(path, key)
			return nil, fmt.Errorf("circular bundle reference detected: %s", strings.Join(cycle, " -> "))
		}
	}

	bundle, err := r.lookupBundle(name, in)
	if err != nil {
		return nil, err
	}

	// Track current path
	currentPath := append(path, key)

	// Collect assets with deduplication
	assetMap := make(map[string]*config.Asset) // assetPath -> asset (later wins)
	order := []string{}                        // Track insertion order

	// First, resolve included bundles. They resolve in this bundle's origin,
	// so an imported bundle's own includes come from the repo that wrote them.
	for _, included := range bundle.Bundles {
		resolved, err := r.resolveEntry(included, in, currentPath)
		if err != nil {
			return nil, fmt.Errorf("resolving included bundle %q: %w", included.Name, err)
		}

		for _, asset := range resolved.Assets {
			assetKey := assetDedupeKey(asset)
			if _, exists := assetMap[assetKey]; !exists {
				order = append(order, assetKey)
			}
			assetMap[assetKey] = asset
		}
	}

	// Then, resolve direct assets (these override included bundle assets)
	for _, entry := range bundle.Assets {
		asset, err := r.resolveAssetRef(entry, in)
		if err != nil {
			return nil, fmt.Errorf("bundle %q: %w", key, err)
		}
		if asset == nil {
			continue // Dropped by the consumer's own source exclusions.
		}

		assetKey := assetDedupeKey(asset)
		if _, exists := assetMap[assetKey]; !exists {
			order = append(order, assetKey)
		}
		assetMap[assetKey] = asset
	}

	// Build result in order
	result := &ResolvedBundle{
		Name:   key,
		Assets: make([]*config.Asset, 0, len(order)),
	}
	for _, k := range order {
		result.Assets = append(result.Assets, assetMap[k])
	}

	return result, nil
}

// lookupBundle finds a bundle by name within an origin.
func (r *Resolver) lookupBundle(name string, in origin) (*config.Bundle, error) {
	bundles, err := r.bundlesIn(in)
	if err != nil {
		return nil, err
	}

	bundle, ok := bundles[name]
	if !ok {
		return nil, fmt.Errorf("bundle not found: %s%s", in.qualify(name), availableBundles(bundles))
	}
	return bundle, nil
}

// bundlesIn returns the bundle map for an origin.
func (r *Resolver) bundlesIn(in origin) (map[string]*config.Bundle, error) {
	if in == activeRepo {
		return r.bundles, nil
	}

	src, ok := r.sources[string(in)]
	if !ok {
		return nil, fmt.Errorf("source not found: %s (in bundle reference)", in)
	}
	return src.LoadBundles()
}

// availableBundles renders a "did you mean" tail for a not-found error. A
// source offering no bundles at all is the more common mistake, so it says so
// outright rather than printing an empty list.
func availableBundles(bundles map[string]*config.Bundle) string {
	if len(bundles) == 0 {
		return " (no bundles defined there)"
	}
	names := make([]string, 0, len(bundles))
	for name := range bundles {
		names = append(names, name)
	}
	sort.Strings(names)
	return " (available: " + strings.Join(names, ", ") + ")"
}

// resolveAssetRef resolves a structured asset entry to an Asset.
//
// It returns a nil asset and no error when the asset is dropped by its own
// source's `exclude:` patterns and the reference came from an imported bundle.
// A reference the user wrote to an excluded asset stays a hard error - that is
// a contradiction within their own config - but a reference upstream wrote is
// one they never made, and failing on it would mean source exclusions and
// bundle imports could not be used together.
func (r *Resolver) resolveAssetRef(entry config.AssetEntry, in origin) (*config.Asset, error) {
	sourceName, assetPath := parseAssetRef(entry.Path, in)

	src, ok := r.sources[sourceName]
	if !ok {
		return nil, fmt.Errorf("source not found: %s (in asset ref %q)%s",
			sourceName, entry.Path, r.describeMissingSource(sourceName, in))
	}

	if in != activeRepo && src.IsExcluded(assetPath) {
		r.logger.Printf("Excluded by source %q: %s (referenced by an imported bundle)", sourceName, assetPath)
		return nil, nil
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

// parseAssetRef splits an asset reference, defaulting an unprefixed path to
// the origin of the bundle that wrote it rather than to "local". For a bundle
// in the active repo that is "local", preserving today's behavior; for an
// imported one it is the repo the bundle came from, which is what its author
// meant.
func parseAssetRef(ref string, in origin) (sourceName, assetPath string) {
	sourceName, assetPath = config.ParseAssetReference(ref)
	if !strings.Contains(ref, ":") && in != activeRepo {
		return string(in), assetPath
	}
	return sourceName, assetPath
}

// describeMissingSource explains an unknown source named by an imported
// bundle, quoting how the origin repo defines it. An imported bundle can
// reference a third source, and the consumer has to declare it under the same
// name; being told what to add beats being told only what is missing.
//
// Best effort: the origin may not be a config repo, or may not be readable.
func (r *Resolver) describeMissingSource(sourceName string, in origin) string {
	if in == activeRepo {
		return ""
	}

	src, ok := r.sources[string(in)]
	if !ok {
		return ""
	}

	taiConfig, err := config.ParseTaiConfig(filepath.Join(src.ContentPath(), "tai.yaml"))
	if err != nil {
		return fmt.Sprintf("; it is referenced by a bundle imported from source %q, whose repo must also be declared in your tai.yaml", in)
	}

	for _, s := range taiConfig.Sources {
		if s.Name != sourceName {
			continue
		}
		if s.IsCommand() {
			return fmt.Sprintf("; the %q repo defines it as a command source - declare a source named %q in your tai.yaml too", in, sourceName)
		}
		return fmt.Sprintf("; the %q repo defines it as path %q - declare a source named %q in your tai.yaml too", in, s.Path, sourceName)
	}

	return fmt.Sprintf("; it is referenced by a bundle imported from source %q - declare a source named %q in your tai.yaml too", in, sourceName)
}

// ResolveMultiple resolves multiple bundles in order, merging their assets.
// Later bundles override earlier ones on conflict (deduplication).
func (r *Resolver) ResolveMultiple(entries []config.BundleEntry) (*ResolvedBundle, []Override, error) {
	assetMap := make(map[string]*config.Asset)
	bundleMap := make(map[string]string) // assetKey -> bundle name that provided it
	order := []string{}
	var overrides []Override
	names := make([]string, 0, len(entries))

	for _, entry := range entries {
		resolved, err := r.resolveEntry(entry, activeRepo, nil)
		if err != nil {
			return nil, nil, err
		}
		names = append(names, resolved.Name)

		for _, asset := range resolved.Assets {
			assetKey := assetDedupeKey(asset)
			if existing, exists := assetMap[assetKey]; exists {
				// Record the override with correct bundle names
				overrides = append(overrides, Override{
					AssetPath:  assetKey,
					FromBundle: bundleMap[assetKey],
					ToBundle:   resolved.Name,
					FromSource: existing.SourceName,
					ToSource:   asset.SourceName,
				})
			} else {
				order = append(order, assetKey)
			}
			assetMap[assetKey] = asset
			bundleMap[assetKey] = resolved.Name
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
