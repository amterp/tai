// Package source handles source loading and asset discovery.
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/cmdexec"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/pathutil"
)

// LoadedSource represents a source with its resolved path and discovered assets.
type LoadedSource struct {
	config.Source
	ResolvedPath string              // Absolute filesystem path
	Assets       map[string][]string // dir name (e.g. "personas") -> list of asset names

	// Materialized reports whether ResolvedPath exists on disk. It is false
	// only for command sources whose fetch has not run yet - Load tolerates
	// that (the config repo must remain loadable before the first fetch),
	// and asset resolution surfaces the actionable error instead.
	Materialized bool

	// ViewPath is the materialized patched copy of this source, set by
	// BuildView. Empty until a view is built, which only happens for sources
	// that declare patches and only during commands that install.
	//
	// ResolvedPath deliberately stays pristine: read-only commands never build
	// a view, so they neither pay for it nor can fail on a broken patch, and
	// `tai locate` keeps pointing at the checkout the user can actually edit
	// rather than at a cache directory the next install replaces.
	ViewPath string
}

// ContentPath is where this source's content should be read from: the patched
// view when one has been built, otherwise the pristine source. Routing every
// content lookup through here is what keeps the rest of tai unaware that
// patches exist.
func (ls *LoadedSource) ContentPath() string {
	if ls.ViewPath != "" {
		return ls.ViewPath
	}
	return ls.ResolvedPath
}

// PristinePath is the unpatched source location. Used by the patch commands,
// which must compare against upstream rather than against tai's own output.
func (ls *LoadedSource) PristinePath() string {
	return ls.ResolvedPath
}

// RequireMaterialized returns an error when the source's content is not on
// disk yet (a command source that hasn't been fetched). Callers resolving
// assets out of a source should check this to give users an actionable
// remedy instead of a bare "asset not found".
func (ls *LoadedSource) RequireMaterialized() error {
	if ls.Materialized {
		return nil
	}
	return fmt.Errorf("source %q has not been fetched yet (command source); run 'tai refresh' or 'tai install <profile>' to fetch it", ls.Name)
}

// Loader handles loading and resolving sources.
type Loader struct {
	configRepoPath string // Path to the tai config repo (for relative path resolution)
	cacheDir       string // Root cache dir (e.g. ~/.cache/tai); required for command sources
}

// NewLoader creates a new source loader.
func NewLoader(configRepoPath string) *Loader {
	return &Loader{configRepoPath: configRepoPath}
}

// SetCacheDir sets the root cache directory command sources materialize
// into. Injected (rather than defaulted) so tests can use temp dirs.
func (l *Loader) SetCacheDir(dir string) {
	l.cacheDir = dir
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
		if src.IsCommand() && os.IsNotExist(err) {
			// A command source with no cache yet must not fail the load -
			// the config repo is loaded eagerly on every CLI invocation,
			// including the install/refresh that would populate the cache.
			// Asset resolution reports the missing fetch instead.
			return &LoadedSource{
				Source:       src,
				ResolvedPath: resolvedPath,
				Assets:       make(map[string][]string),
			}, nil
		}
		return nil, fmt.Errorf("source path does not exist: %s", resolvedPath)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source path is not a directory: %s", resolvedPath)
	}

	ls := &LoadedSource{
		Source:       src,
		ResolvedPath: resolvedPath,
		Assets:       make(map[string][]string),
		Materialized: true,
	}

	// Discover assets
	if err := l.discoverAssets(ls); err != nil {
		return nil, fmt.Errorf("discovering assets: %w", err)
	}

	return ls, nil
}

// resolvePath resolves the source path to an absolute filesystem path.
func (l *Loader) resolvePath(src config.Source) (string, error) {
	if src.IsCommand() {
		return l.sourceCachePath(src)
	}

	path, err := pathutil.MakeAbsolute(src.Path, l.configRepoPath)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	// For tai-repo sources, point to assets/ subdirectory
	if src.Type == "tai-repo" {
		path = filepath.Join(path, "assets")
	}

	return path, nil
}

// sourceCachePath returns the cache directory a command source materializes
// into: <cacheDir>/sources/<repoKey>/<name>. The repoKey namespaces caches
// per config repo - two repos can declare same-named command sources with
// different commands, and switching the active config must not let one
// repo's fetch poison another's.
func (l *Loader) sourceCachePath(src config.Source) (string, error) {
	if l.cacheDir == "" {
		return "", fmt.Errorf("source %q: no cache directory configured for command sources", src.Name)
	}
	return filepath.Join(l.cacheDir, "sources", l.repoKey(), src.Name), nil
}

func (l *Loader) repoKey() string {
	sum := sha256.Sum256([]byte(l.configRepoPath))
	return hex.EncodeToString(sum[:])[:12]
}

// Refresh materializes a command source by running its fetch command. The
// command writes into a staging directory (exposed as TAI_DEST_DIR) that is
// atomically swapped into the source's cache path on success; on failure the
// previous cache content is left intact. version is surfaced to the script
// as TAI_VERSION.
func (l *Loader) Refresh(src config.Source, version string) error {
	if !src.IsCommand() {
		return fmt.Errorf("source %q is not a command source", src.Name)
	}
	cachePath, err := l.sourceCachePath(src)
	if err != nil {
		return err
	}

	// Stage next to the cache path (same filesystem) so the swap is a true
	// atomic rename rather than the cross-device copy fallback.
	stagingParent := filepath.Join(l.cacheDir, "staging")

	err = cmdexec.FetchInto(cmdexec.Spec{
		Argv:    src.Command,
		Timeout: src.ResolvedTimeout(),
		Env: map[string]string{
			"TAI_SOURCE":      src.Name,
			"TAI_CONFIG_REPO": l.configRepoPath,
			"TAI_VERSION":     version,
		},
	}, cachePath, stagingParent)
	if err != nil {
		return fmt.Errorf("fetching source %q: %w", src.Name, err)
	}
	return nil
}

// discoverAssets discovers all assets in a loaded source.
func (l *Loader) discoverAssets(ls *LoadedSource) error {
	for _, assetType := range config.ValidAssetTypes() {
		dirName := config.AssetTypeDirName(assetType)
		assetDir := filepath.Join(ls.ResolvedPath, dirName)
		assets, err := l.discoverAssetsInDir(assetDir, assetType)
		if err != nil {
			return err
		}

		kept := assets[:0]
		for _, name := range assets {
			if !ls.IsExcluded(dirName + "/" + name) {
				kept = append(kept, name)
			}
		}
		ls.Assets[dirName] = kept
	}
	return nil
}

// discoverAssetsInDir discovers assets in a specific asset type directory.
// Assets can be files OR directories. For directories, we check for the
// conventional main file (PERSONA.md, DELEGATE.md, SKILL.md, config.yaml).
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

		if entry.IsDir() {
			// Directory asset - check if this type supports directories
			mainFile := config.MainFileName(assetType)
			if mainFile == "" {
				// This type doesn't support directories (e.g., "file" type)
				continue
			}

			// Verify the main file exists
			mainFilePath := filepath.Join(dir, name, mainFile)
			if _, err := os.Stat(mainFilePath); err == nil {
				assets = append(assets, name)
			}
			// If main file doesn't exist, silently skip this directory.
			// This is intentional: random directories in assets/ are ignored
			// rather than causing errors. However, if a user explicitly
			// references this directory in a bundle, resolution will fail
			// with a clear "asset not found" error.
		} else {
			// File asset - all types support files (except we skip directories above)
			assets = append(assets, name)
		}
	}

	return assets, nil
}

// GetAsset returns the full path to an asset.
// Returns false if the path escapes the source directory (path traversal
// protection), or if the source's `exclude:` patterns drop it.
func (ls *LoadedSource) GetAsset(assetPath string) (string, bool) {
	// Excluded paths still exist on disk, so this check has to be explicit -
	// otherwise an exclusion would filter listings while leaving the file
	// reachable by anyone who names it directly.
	if ls.IsExcluded(assetPath) {
		return "", false
	}

	// assetPath is like "personas/senior-swe.md" or "skills/my-skill"
	base := ls.ContentPath()
	fullPath := filepath.Join(base, assetPath)

	// Verify the result is still within the source directory
	if !pathutil.IsContainedIn(fullPath, base) {
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

// ResolveAsset resolves an asset path with an explicit type to a config.Asset.
// The assetPath is the filesystem path within the source (e.g. "personas/senior-swe.md").
// The assetType is the singular type (e.g. "persona").
func (ls *LoadedSource) ResolveAsset(assetPath, assetType string) (*config.Asset, error) {
	if err := ls.RequireMaterialized(); err != nil {
		return nil, err
	}

	fullPath, ok := ls.GetAsset(assetPath)
	if !ok {
		// An excluded asset is usually sitting right there on disk, so a bare
		// "not found" would send the user looking at the source instead of at
		// their own exclude list.
		if ls.IsExcluded(assetPath) {
			return nil, fmt.Errorf("asset %s is excluded by source %q's 'exclude' patterns; remove the exclusion or stop referencing the asset", assetPath, ls.Name)
		}
		return nil, fmt.Errorf("asset not found: %s", assetPath)
	}

	if !config.IsValidAssetType(assetType) {
		return nil, fmt.Errorf("invalid asset type: %s", assetType)
	}

	assetName := filepath.Base(assetPath)

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
	for dirName, names := range ls.Assets {
		for _, name := range names {
			all = append(all, filepath.Join(dirName, name))
		}
	}
	return all
}
