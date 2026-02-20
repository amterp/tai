// Package state manages tai's global state and configuration.
package state

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/amterp/tai/internal/adapter"
	_ "github.com/amterp/tai/internal/adapter/claude"   // Register Claude adapter
	_ "github.com/amterp/tai/internal/adapter/codex"    // Register Codex adapter
	_ "github.com/amterp/tai/internal/adapter/opencode" // Register OpenCode adapter
	"github.com/amterp/tai/internal/bundle"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
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

// Load loads the full tai state.
func Load() (*State, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return nil, err
	}

	globalCfg, err := LoadGlobalConfig(paths)
	if err != nil {
		return nil, err
	}

	manifestStore := install.NewManifestStore(paths.ManifestDir)
	state := &State{
		Paths:         paths,
		GlobalConfig:  globalCfg,
		ManifestStore: manifestStore,
		Installer:     install.NewInstaller(manifestStore, "", nil), // Empty config repo path and no sources for now
	}

	// If we have an active config, load it
	if globalCfg.ActiveConfig != "" {
		if err := state.LoadConfigRepo(globalCfg.ActiveConfig); err != nil {
			return nil, err
		}
	}

	return state, nil
}

// LoadConfigRepo loads a config repository.
func (s *State) LoadConfigRepo(path string) error {
	// Expand ~ and make absolute
	absPath, err := pathutil.MakeAbsolute(path, "")
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	// MakeAbsolute with empty base won't make relative paths absolute,
	// so we need to handle that case
	if !filepath.IsAbs(absPath) {
		absPath, err = filepath.Abs(absPath)
		if err != nil {
			return fmt.Errorf("making path absolute: %w", err)
		}
	}

	// Parse tai.yaml
	taiConfig, err := config.ParseTaiConfig(filepath.Join(absPath, "tai.yaml"))
	if err != nil {
		return err
	}
	s.TaiConfig = taiConfig

	// Load sources
	loader := source.NewLoader(absPath)
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
		if _, err := adapter.DefaultRegistry.Get(profile.Target); err != nil {
			return fmt.Errorf("profile %q: unknown target %q (no adapter registered)", name, profile.Target)
		}
	}

	// Create resolvers
	s.BundleResolver = bundle.NewResolver(bundles, sources)
	s.ProfileResolver = profile.NewResolver(profiles, bundles, sources)

	// Create installer with sources for include resolution
	s.Installer = install.NewInstaller(s.ManifestStore, absPath, sources)

	return nil
}

// SetActiveConfig sets the active config repo path and saves it.
func (s *State) SetActiveConfig(path string) error {
	// Expand ~ and make absolute
	absPath, err := pathutil.MakeAbsolute(path, "")
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}
	if !filepath.IsAbs(absPath) {
		absPath, err = filepath.Abs(absPath)
		if err != nil {
			return fmt.Errorf("making path absolute: %w", err)
		}
	}

	// Verify tai.yaml exists
	taiYaml := filepath.Join(absPath, "tai.yaml")
	if _, err := os.Stat(taiYaml); os.IsNotExist(err) {
		return fmt.Errorf("not a valid config repo: %s (tai.yaml not found)", absPath)
	}

	s.GlobalConfig.ActiveConfig = absPath
	if err := SaveGlobalConfig(s.Paths, s.GlobalConfig); err != nil {
		return err
	}

	// Load the config repo
	return s.LoadConfigRepo(absPath)
}

// RequireActiveConfig returns an error if no config is active.
func (s *State) RequireActiveConfig() error {
	if s.GlobalConfig.ActiveConfig == "" {
		return fmt.Errorf("no active config repo. Use 'tai config set <path>' to set one")
	}
	return nil
}

// GetAdapter returns an adapter by name.
func (s *State) GetAdapter(name string) (adapter.Adapter, error) {
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

	fullPath, ok := src.GetAsset(path)
	if !ok {
		return "", fmt.Errorf("asset not found: %s in source %s", path, sourceName)
	}

	return fullPath, nil
}
