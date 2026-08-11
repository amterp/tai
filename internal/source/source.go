// Package source handles source loading and asset discovery.
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/amterp/tai/internal/cmdexec"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/fetch"
	"github.com/amterp/tai/internal/pathutil"
)

// LoadedSource represents a source with its resolved path and discovered assets.
type LoadedSource struct {
	config.Source
	ResolvedPath string // Absolute filesystem path to the source root

	// Assets lists every discovered asset as a root-relative path
	// (e.g. "assets/skills/rad") - the same form a bundle entry names, so
	// `tai list assets` output can be pasted straight into one.
	Assets []string

	// Materialized reports whether ResolvedPath exists on disk. It is false
	// only for fetched sources whose fetch has not run yet - Load tolerates
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

	// bundles caches LoadBundles' result. Loading is lazy rather than part of
	// Load: the config repo is loaded on every CLI invocation, and a malformed
	// bundle in a source nobody imports from should not break every command.
	bundles     map[string]*config.Bundle
	bundlesErr  error
	bundlesRead bool
}

// BundlesDirName is where a source keeps the bundles it offers for import.
// Bundles are addressed by name rather than by path, so unlike assets this
// directory is a convention tai knows rather than something a reference spells
// out - a bundle's identity is its `name:` field, not its filename.
const BundlesDirName = "bundles"

// BundlesPath is the directory this source's importable bundles live in.
func (ls *LoadedSource) BundlesPath() string {
	return filepath.Join(ls.ContentPath(), BundlesDirName)
}

// LoadBundles returns the bundles this source offers for import, keyed by
// name. A source with no bundles/ directory returns an empty map rather than
// an error: offering no bundles is ordinary, and the resolver reports the
// missing bundle by name, which is the more useful message.
func (ls *LoadedSource) LoadBundles() (map[string]*config.Bundle, error) {
	if ls.bundlesRead {
		return ls.bundles, ls.bundlesErr
	}
	ls.bundlesRead = true

	if err := ls.RequireMaterialized(); err != nil {
		ls.bundlesErr = err
		return nil, err
	}

	bundles, err := config.LoadBundlesFromDir(ls.BundlesPath())
	if err != nil {
		ls.bundlesErr = fmt.Errorf("loading bundles from source %q: %w", ls.Name, err)
		return nil, ls.bundlesErr
	}

	ls.bundles = bundles
	return bundles, nil
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
// disk yet (a fetched source whose fetch hasn't run). Callers resolving
// assets out of a source should check this to give users an actionable
// remedy instead of a bare "asset not found".
func (ls *LoadedSource) RequireMaterialized() error {
	if ls.Materialized {
		return nil
	}
	return fmt.Errorf("source %q has not been fetched yet (%s source); run 'tai refresh' or 'tai install <profile>' to fetch it", ls.Name, ls.Type)
}

// Loader handles loading and resolving sources.
type Loader struct {
	configRepoPath string // Path to the tai config repo (for relative path resolution)
	cacheDir       string // Root cache dir (e.g. ~/.cache/tai); required for fetched sources
}

// NewLoader creates a new source loader.
func NewLoader(configRepoPath string) *Loader {
	return &Loader{configRepoPath: configRepoPath}
}

// SetCacheDir sets the root cache directory fetched sources materialize
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
		if src.NeedsFetch() && os.IsNotExist(err) {
			// A fetched source with no cache yet must not fail the load -
			// the config repo is loaded eagerly on every CLI invocation,
			// including the install/refresh that would populate the cache.
			// Asset resolution reports the missing fetch instead.
			return &LoadedSource{
				Source:       src,
				ResolvedPath: resolvedPath,
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
		Materialized: true,
	}

	// Discover assets
	if err := l.discoverAssets(ls); err != nil {
		return nil, fmt.Errorf("discovering assets: %w", err)
	}

	return ls, nil
}

// resolvePath resolves the source path to an absolute filesystem path.
//
// A source is a plain directory root and nothing is appended to it: every
// reference names a literal path from here, so `assets/skills/rad` reaches
// exactly that. Interpreting the path per source type is what made bundles
// unreachable, since they sit outside whatever tai decided the asset root was.
func (l *Loader) resolvePath(src config.Source) (string, error) {
	if src.NeedsFetch() {
		return l.sourceCachePath(src)
	}

	path, err := pathutil.MakeAbsolute(src.Path, l.configRepoPath)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	return path, nil
}

// sourceCachePath returns the cache directory a fetched source materializes
// into: <cacheDir>/sources/<repoKey>/<name>. The repoKey namespaces caches
// per config repo - two repos can declare same-named fetched sources with
// different commands or URLs, and switching the active config must not let
// one repo's fetch poison another's.
func (l *Loader) sourceCachePath(src config.Source) (string, error) {
	if l.cacheDir == "" {
		return "", fmt.Errorf("source %q: no cache directory configured for fetched sources", src.Name)
	}
	return filepath.Join(l.cacheDir, "sources", l.repoKey(), src.Name), nil
}

func (l *Loader) repoKey() string {
	sum := sha256.Sum256([]byte(l.configRepoPath))
	return hex.EncodeToString(sum[:])[:12]
}

// Refresh materializes a fetched source into its cache path. A command source
// runs its argv against a staging directory (exposed as TAI_DEST_DIR); a url
// source downloads its single file into one. Either way the staging directory
// is atomically swapped into the cache path on success, and on failure the
// previous cache content is left intact. version is surfaced to command
// scripts as TAI_VERSION and to url fetches as the User-Agent.
func (l *Loader) Refresh(src config.Source, version string) error {
	if !src.NeedsFetch() {
		return fmt.Errorf("source %q cannot be fetched (type %q); only 'command' and 'url' sources are fetched", src.Name, src.Type)
	}
	cachePath, err := l.sourceCachePath(src)
	if err != nil {
		return err
	}

	// Stage next to the cache path (same filesystem) so the swap is a true
	// atomic rename rather than the cross-device copy fallback.
	stagingParent := filepath.Join(l.cacheDir, "staging")

	if src.IsURL() {
		if err := l.fetchURLSource(src, cachePath, stagingParent, version); err != nil {
			return fmt.Errorf("fetching source %q: %w", src.Name, err)
		}
		return nil
	}

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

// fetchURLSource downloads a url source's single file into a staging
// directory and swaps that directory into cachePath.
//
// It builds a directory rather than writing the file straight to its final
// path for two reasons: a url source is modelled as a one-file directory so
// everything downstream can walk it like any other source, and staging plus
// swap is what makes a failed fetch leave the previous content intact - the
// same contract cmdexec.FetchInto gives command sources.
func (l *Loader) fetchURLSource(src config.Source, cachePath, stagingParent, version string) error {
	if err := os.MkdirAll(stagingParent, 0755); err != nil {
		return fmt.Errorf("creating staging parent: %w", err)
	}
	staging, err := os.MkdirTemp(stagingParent, "tai-url-")
	if err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	client := &http.Client{
		Timeout:       src.ResolvedTimeout(),
		CheckRedirect: fetch.DefaultClient().CheckRedirect,
	}
	userAgent := "tai"
	if version != "" {
		userAgent = "tai/" + version
	}

	dest := filepath.Join(staging, src.ResolvedFilename())
	if err := fetch.ToFile(client, src.URL, dest, userAgent); err != nil {
		return err
	}

	return cmdexec.SwapInto(staging, cachePath)
}

// assetSearchRoots are the source-relative directories under which tai looks
// for asset type directories when listing a source's contents.
//
// Both layouts exist: a tai config repo keeps its assets under assets/, while a
// plain directory of assets has the type dirs at its top level. Scanning both
// is not an ambiguity to resolve, because nothing here feeds resolution - a
// bundle entry names a literal path from the source root, and each result is
// reported as exactly that path. This only decides what `tai list assets`
// enumerates.
var assetSearchRoots = []string{"", "assets"}

// discoverAssets discovers all assets in a loaded source, as root-relative
// paths.
func (l *Loader) discoverAssets(ls *LoadedSource) error {
	var found []string
	for _, searchRoot := range assetSearchRoots {
		for _, assetType := range config.ValidAssetTypes() {
			relDir := path.Join(searchRoot, config.AssetTypeDirName(assetType))
			names, err := l.discoverAssetsInDir(filepath.Join(ls.ResolvedPath, relDir), assetType)
			if err != nil {
				return err
			}

			for _, name := range names {
				rel := relDir + "/" + name
				if !ls.IsExcluded(rel) {
					found = append(found, rel)
				}
			}
		}
	}
	sort.Strings(found)
	ls.Assets = found
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
		RelPath:    filepath.ToSlash(assetPath),
		IsDir:      info.IsDir(),
	}, nil
}

// ListAllAssets returns every discovered asset as a root-relative path.
// The result is a copy, so callers are free to sort or filter it.
func (ls *LoadedSource) ListAllAssets() []string {
	return slices.Clone(ls.Assets)
}
