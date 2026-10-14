// Package state manages tai's global state and configuration.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/amterp/tai/internal/adapter"
	_ "github.com/amterp/tai/internal/adapter/claude"   // Register Claude adapter
	_ "github.com/amterp/tai/internal/adapter/codex"    // Register Codex adapter
	_ "github.com/amterp/tai/internal/adapter/opencode" // Register OpenCode adapter
	"github.com/amterp/tai/internal/bundle"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/patch"
	"github.com/amterp/tai/internal/pathutil"
	"github.com/amterp/tai/internal/profile"
	"github.com/amterp/tai/internal/source"
)

// GlobalConfig holds tai's global CLI configuration.
type GlobalConfig struct {
	ActiveConfig string `yaml:"active_config"` // Path to active config repo
}

// Paths holds the various paths used by tai.
type Paths struct {
	ConfigDir   string // ~/.config/tai
	CacheDir    string // ~/.cache/tai
	ManifestDir string // ~/.config/tai/manifests
	ConfigFile  string // ~/.config/tai/config.yaml
}

// DefaultPaths returns the default paths for tai.
func DefaultPaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", "tai")
	return &Paths{
		ConfigDir:   configDir,
		CacheDir:    filepath.Join(home, ".cache", "tai"),
		ManifestDir: filepath.Join(configDir, "manifests"),
		ConfigFile:  filepath.Join(configDir, "config.yaml"),
	}, nil
}

// State holds the loaded state of tai.
type State struct {
	Paths        *Paths
	GlobalConfig *GlobalConfig

	// Loaded from active config repo
	TaiConfig *config.TaiConfig
	Sources   map[string]*source.LoadedSource
	Bundles   map[string]*config.Bundle
	Profiles  map[string]*config.Profile

	// Resolvers
	BundleResolver  *bundle.Resolver
	ProfileResolver *profile.Resolver

	// Installation
	ManifestStore *install.ManifestStore
	Installer     *install.Installer

	// Version is the tai binary's version string. Propagated to the Installer
	// (and any future Installer rebuilt by LoadConfigRepo) so command-based
	// includes can surface it to scripts via TAI_VERSION.
	Version string
}

// SetVersion sets the tai version string and propagates it to the current
// Installer. Call this early - before any install runs - so subsequent
// installers (e.g. created by LoadConfigRepo) also pick it up.
func (s *State) SetVersion(v string) {
	s.Version = v
	if s.Installer != nil {
		s.Installer.SetVersion(v)
	}
}

// LoadGlobalConfig loads the global config file.
func LoadGlobalConfig(paths *Paths) (*GlobalConfig, error) {
	data, err := os.ReadFile(paths.ConfigFile)
	if os.IsNotExist(err) {
		return &GlobalConfig{}, nil // No config yet
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

// SaveGlobalConfig saves the global config file.
func SaveGlobalConfig(paths *Paths, cfg *GlobalConfig) error {
	if err := os.MkdirAll(paths.ConfigDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}

	if err := os.WriteFile(paths.ConfigFile, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// Load reads tai's global config and loads the active config repo, if one is
// set. Commands that only need the global config use New instead, so a repo
// that no longer loads cannot lock them out.
func Load() (*State, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return nil, err
	}
	s, err := New(paths)
	if err != nil {
		return nil, err
	}
	if err := s.LoadConfiguredRepo(); err != nil {
		return nil, err
	}
	return s, nil
}

// New reads tai's global config without loading any config repo.
func New(paths *Paths) (*State, error) {
	globalCfg, err := LoadGlobalConfig(paths)
	if err != nil {
		return nil, err
	}

	manifestStore := install.NewManifestStore(paths.ManifestDir)
	return &State{
		Paths:         paths,
		GlobalConfig:  globalCfg,
		ManifestStore: manifestStore,
		Installer:     install.NewInstaller(manifestStore, "", nil),
	}, nil
}

// LoadConfiguredRepo loads the active config repo, or does nothing when none
// is set.
func (s *State) LoadConfiguredRepo() error {
	active := s.GlobalConfig.ActiveConfig
	if active == "" {
		return nil
	}
	if err := s.LoadConfigRepo(active); err != nil {
		return fmt.Errorf("active config repo %s failed to load: %w. Fix the repo, or switch to another with "+
			"'tai config set <path>', which works even while this one is broken", active, err)
	}
	return nil
}

// absRepoPath expands ~ and makes a config repo path absolute. Every path a
// repo is known by goes through here, since the source cache is keyed by the
// repo's path string.
func absRepoPath(path string) (string, error) {
	absPath, err := pathutil.MakeAbsolute(path, "")
	if err != nil {
		return "", fmt.Errorf("resolving config path: %w", err)
	}
	// MakeAbsolute with an empty base leaves a relative path relative.
	if !filepath.IsAbs(absPath) {
		absPath, err = filepath.Abs(absPath)
		if err != nil {
			return "", fmt.Errorf("making path absolute: %w", err)
		}
	}
	return absPath, nil
}

// LoadConfigRepo loads a config repository.
func (s *State) LoadConfigRepo(path string) error {
	absPath, err := absRepoPath(path)
	if err != nil {
		return err
	}

	// Parse tai.yaml
	taiConfig, err := config.ParseTaiConfig(filepath.Join(absPath, "tai.yaml"))
	if err != nil {
		return err
	}
	s.TaiConfig = taiConfig

	// Validated here rather than in config, which cannot see the adapter list.
	for _, src := range taiConfig.Sources {
		target := src.NativeFrontmatter
		if target == "" || slices.Contains(adapter.TargetNames(), target) {
			continue
		}
		if advice, removed := adapter.RemovedTarget(target); removed {
			return fmt.Errorf("source %q: native_frontmatter names %q, which is no longer supported. %s", src.Name, target, advice)
		}
		return fmt.Errorf("source %q: native_frontmatter %q is not a target tai supports (known targets: %s)",
			src.Name, target, strings.Join(adapter.TargetNames(), ", "))
	}

	// Load sources
	loader := source.NewLoader(absPath)
	loader.SetCacheDir(s.Paths.CacheDir)
	sources, err := loader.LoadAll(taiConfig.Sources)
	if err != nil {
		return err
	}
	s.Sources = sources

	// Load bundles
	bundles, err := config.LoadBundlesFromDir(filepath.Join(absPath, "bundles"))
	if err != nil {
		return err
	}
	s.Bundles = bundles

	// Load profiles
	profiles, err := config.LoadProfilesFromDir(filepath.Join(absPath, "profiles"))
	if err != nil {
		return err
	}
	s.Profiles = profiles

	// Validate profile targets against registered adapters
	for name, profile := range profiles {
		if advice, removed := adapter.RemovedTarget(profile.Target); removed {
			return fmt.Errorf("profile %q: target %q is no longer supported. %s", name, profile.Target, advice)
		}
		if _, err := adapter.DefaultRegistry.Get(profile.Target); err != nil {
			return fmt.Errorf("profile %q: unknown target %q (no adapter registered)", name, profile.Target)
		}
	}

	// Create resolvers
	s.BundleResolver = bundle.NewResolver(bundles, sources)
	s.ProfileResolver = profile.NewResolver(profiles, bundles, sources)

	// Create installer with sources for include resolution
	s.Installer = install.NewInstaller(s.ManifestStore, absPath, sources)
	if s.Version != "" {
		s.Installer.SetVersion(s.Version)
	}

	return nil
}

// HasFetchableSources reports whether the active config declares any source
// whose content is pulled into the cache.
func (s *State) HasFetchableSources() bool {
	if s.TaiConfig == nil {
		return false
	}
	for _, src := range s.TaiConfig.Sources {
		if src.NeedsFetch() {
			return true
		}
	}
	return false
}

// RefreshSources fetches every source that needs fetching (or just the named
// one when name is non-empty), then reloads the config repo so sources,
// resolvers, and the installer all see the freshly materialized content -
// they capture the sources map at construction, so mutating it in place would
// leave them stale. onRefresh (may be nil) is called with each source name
// before its fetch runs, for progress output.
func (s *State) RefreshSources(name string, onRefresh func(name string)) error {
	if err := s.RequireActiveConfig(); err != nil {
		return err
	}

	loader := source.NewLoader(s.GlobalConfig.ActiveConfig)
	loader.SetCacheDir(s.Paths.CacheDir)

	refreshed := 0
	for _, src := range s.TaiConfig.Sources {
		if !src.NeedsFetch() {
			if name != "" && src.Name == name {
				return fmt.Errorf("source %q does not need fetching (type %q); only 'command' and 'url' sources are fetched", src.Name, src.Type)
			}
			continue
		}
		if name != "" && src.Name != name {
			continue
		}
		if onRefresh != nil {
			onRefresh(src.Name)
		}
		if err := loader.Refresh(src, s.Version); err != nil {
			return err
		}
		refreshed++
	}

	if name != "" && refreshed == 0 {
		return fmt.Errorf("source not found: %s", name)
	}
	if refreshed == 0 {
		return nil
	}

	return s.LoadConfigRepo(s.GlobalConfig.ActiveConfig)
}

// LoadPatches loads the patch set a source declares, or an empty set when it
// declares none.
func (s *State) LoadPatches(src *source.LoadedSource) (*patch.Set, error) {
	if !src.HasPatches() {
		return patch.NewSet(src.Name, nil), nil
	}
	dir := filepath.Join(s.GlobalConfig.ActiveConfig, filepath.FromSlash(src.Patches))
	return patch.LoadDir(dir, src.Name)
}

// BuildPatchedViews materializes a patched view for every source that declares
// patches, and points those sources at their views.
//
// Called once per command invocation before installing, rather than during
// config load: loading happens on every CLI invocation, so building there would
// make `tai list` pay for materialization and let a broken patch fail a
// read-only command. Building once here also means a multi-target reinstall
// builds each view a single time and shares it across targets.
func (s *State) BuildPatchedViews() error {
	if err := s.RequireActiveConfig(); err != nil {
		return err
	}

	loader := s.patchLoader()

	for _, src := range s.Sources {
		if !src.HasPatches() {
			continue
		}
		patches, err := s.LoadPatches(src)
		if err != nil {
			return err
		}
		if err := loader.BuildView(src, patches); err != nil {
			return err
		}
	}
	return nil
}

// BuildPatchWorkspace materializes an editable copy of a source for authoring
// or repairing patches.
func (s *State) BuildPatchWorkspace(src *source.LoadedSource, patches *patch.Set) (string, []string, error) {
	return s.patchLoader().BuildWorkspace(src, patches)
}

// PatchWorkspacePath is where a source's patch workspace lives.
func (s *State) PatchWorkspacePath(sourceName string) (string, error) {
	return s.patchLoader().WorkspacePath(sourceName)
}

func (s *State) patchLoader() *source.Loader {
	loader := source.NewLoader(s.GlobalConfig.ActiveConfig)
	loader.SetCacheDir(s.Paths.CacheDir)
	return loader
}

// SetActiveConfig loads the config repo at path and, only if it loads, saves it
// as the active config.
//
// Loading first is the point: saving first would leave a repo that does not
// load on this machine active, failing every later command that reads it.
func (s *State) SetActiveConfig(path string) error {
	absPath, err := absRepoPath(path)
	if err != nil {
		return err
	}

	taiYaml := filepath.Join(absPath, "tai.yaml")
	if _, err := os.Stat(taiYaml); os.IsNotExist(err) {
		return fmt.Errorf("not a valid config repo: %s (tai.yaml not found)", absPath)
	}

	if err := s.LoadConfigRepo(absPath); err != nil {
		return fmt.Errorf("not switching to %s, which failed to load (active config is still %s): %w",
			absPath, describeActive(s.GlobalConfig.ActiveConfig), err)
	}

	s.GlobalConfig.ActiveConfig = absPath
	return SaveGlobalConfig(s.Paths, s.GlobalConfig)
}

func describeActive(path string) string {
	if path == "" {
		return "unset"
	}
	return path
}

// RequireActiveConfig returns an error if no config is active.
func (s *State) RequireActiveConfig() error {
	if s.GlobalConfig.ActiveConfig == "" {
		return fmt.Errorf("no active config repo. Use 'tai config set <path>' to set one")
	}
	return nil
}

// GetAdapter returns an adapter by name. A target tai dropped gets its
// removal advice rather than the "unknown target" a typo gets.
func (s *State) GetAdapter(name string) (adapter.Adapter, error) {
	if advice, removed := adapter.RemovedTarget(name); removed {
		return nil, fmt.Errorf("target %q is no longer supported. %s", name, advice)
	}
	return adapter.DefaultRegistry.Get(name)
}

// ResolveProfile resolves a profile by name.
func (s *State) ResolveProfile(name string) (*profile.ResolvedProfile, error) {
	if err := s.RequireActiveConfig(); err != nil {
		return nil, err
	}
	return s.ProfileResolver.Resolve(name)
}

// LocateAsset finds the filesystem path to an asset.
func (s *State) LocateAsset(assetPath string) (string, error) {
	if err := s.RequireActiveConfig(); err != nil {
		return "", err
	}

	sourceName, path := config.ParseAssetReference(assetPath)

	src, ok := s.Sources[sourceName]
	if !ok {
		return "", fmt.Errorf("source not found: %s", sourceName)
	}

	if err := src.RequireMaterialized(); err != nil {
		return "", err
	}

	fullPath, ok := src.GetAsset(path)
	if !ok {
		return "", fmt.Errorf("asset not found: %s in source %s", path, sourceName)
	}

	return fullPath, nil
}
