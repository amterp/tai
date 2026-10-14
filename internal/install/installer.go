package install

import (
	"fmt"
	"maps"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
	"github.com/amterp/tai/internal/fetch"
	"github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/pathutil"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/template"
)

const (
	// SkillManifestFilename is the name of the skill manifest file.
	// This file is a build-time artifact and should not be installed.
	SkillManifestFilename = "tai.yaml"
)

// InstallOptions configures installation behavior.
type InstallOptions struct {
	DryRun        bool // Preview without making changes
	Force         bool // Overwrite conflicting files
	SkipConflicts bool // Leave conflicting files as-is
	// Variables holds user-provided template variables (tai.yaml + profile + CLI merged).
	// Adapter vars are prepended at lowest priority by buildInstallContext - do not
	// include them here, or they will be merged twice (harmlessly but wastefully).
	Variables template.Variables

	// Scope is the profile's default installation scope for config assets.
	// Empty means config.DefaultScope. Individual assets may override it.
	Scope string

	// Consent is asked before tai writes a config file that requires the
	// user's agreement. Returning false skips those assets with a warning.
	//
	// Nil declines everything, which is the right default for a library caller
	// and for any non-interactive context: nothing that needs asking about
	// happens without someone having been asked.
	Consent func(configasset.ConsentRequest) (bool, error)
}

// InstallResult contains the result of an installation.
type InstallResult struct {
	Added     []string // Files added (tai was not managing them before)
	Restored  []string // Files tai manages that were gone from the target and got written back
	Updated   []string // Files updated (content changed)
	Unchanged []string // Files unchanged (content identical, and already present)
	Removed   []string // Files removed
	Skipped   []string // Files skipped due to conflicts
	Errors    []error  // Non-fatal errors
	Warnings  []string // Non-fatal warnings (e.g., unsupported asset types)
	Manifest  *Manifest
}

// Logger is an interface for logging messages during installation.
type Logger interface {
	Printf(format string, args ...any)
}

// Conflict represents a file conflict.
type Conflict struct {
	Path   string
	Reason string // "user_modified" or "unmanaged"
}

// Installer handles installing assets to targets.
type Installer struct {
	manifestStore *ManifestStore
	configRepo    string
	sources       map[string]*source.LoadedSource
	httpClient    *http.Client
	logger        Logger
	version       string // exposed to command-based includes via TAI_VERSION
}

// noopLogger discards all log messages.
type noopLogger struct{}

func (noopLogger) Printf(string, ...any) {}

// NewInstaller creates a new installer.
func NewInstaller(manifestStore *ManifestStore, configRepo string, sources map[string]*source.LoadedSource) *Installer {
	return &Installer{
		manifestStore: manifestStore,
		configRepo:    configRepo,
		sources:       sources,
		httpClient:    fetch.DefaultClient(),
		logger:        noopLogger{},
	}
}

// SetLogger sets the logger for the installer.
func (i *Installer) SetLogger(logger Logger) {
	i.logger = logger
}

// SetVersion sets the tai version string surfaced to command-based skill
// includes via the TAI_VERSION env var. Optional; unset means scripts see no
// TAI_VERSION.
func (i *Installer) SetVersion(v string) {
	i.version = v
}

// resolvedFileAsset holds per-asset state during the two-phase install process.
type resolvedFileAsset struct {
	asset         *config.Asset
	workingAsset  *config.Asset // May differ from asset if dest was templated
	targetPath    string
	fullPath      string
	resolved      *config.ResolvedAsset
	existingEntry ManifestEntry
	wasManaged    bool

	// existsOnDisk records whether the target was there before this install.
	// Classification cannot be read off the manifest alone: a file the manifest
	// claims is installed but that has since been deleted has the recorded
	// checksum, so it looks identical to one nobody touched.
	existsOnDisk bool

	conflict *Conflict
}

// installContext holds shared state during the two-phase install process.
// This avoids threading many parameters through helper functions.
type installContext struct {
	adap           adapter.Adapter
	targetDir      string
	templateEngine *template.Engine

	// untemplated names the sources that declared `template: false`.
	untemplated   map[string]bool
	existingFiles map[string]ManifestEntry

	// existingConfigEntries is keyed by ConfigEntryKey, not by asset type.
	// Collection types (hooks, MCP servers) hold many entries per type, so
	// keying by type alone would silently collapse them to one.
	existingConfigEntries map[string]ConfigEntry

	// configFiles batches every target config file touched this install, so
	// each is read and written once even when many assets share it.
	configFiles *configFileSet

	// consent caches answers by request key, so a profile with several assets
	// writing the same file asks about it once.
	consent map[string]bool

	opts InstallOptions
}

// resolvedConfigAsset pairs a config asset with the scope it resolved to.
type resolvedConfigAsset struct {
	asset *config.Asset
	scope string

	// vanished records that tai's entry was missing from the config file before
	// this install. Same blind spot as existsOnDisk on file assets: the manifest
	// still holds the entry and its checksum, so re-adding it is otherwise
	// indistinguishable from leaving it alone.
	vanished bool
}

// scopeFor resolves an asset's installation scope: its own override, else the
// profile default, else tai's default.
func (ctx *installContext) scopeFor(asset *config.Asset) string {
	if asset.Scope != "" {
		return asset.Scope
	}
	if ctx.opts.Scope != "" {
		return ctx.opts.Scope
	}
	return config.DefaultScope
}

// Install installs resolved assets to a target.
//
// The install process uses two phases:
//   - Phase 1: Resolve all assets and detect conflicts upfront
//   - Phase 2: Install assets (only if no conflicts, or force/skip flags set)
//
// This ensures all conflicts are reported at once rather than failing on the first one.
func (i *Installer) Install(
	adap adapter.Adapter,
	profile string,
	assets []*config.Asset,
	opts InstallOptions,
) (*InstallResult, error) {
	// Load existing manifest
	existing, err := i.manifestStore.Load(adap.Name())
	if err != nil {
		return nil, fmt.Errorf("loading existing manifest: %w", err)
	}

	// Build context for helper functions
	ctx := i.buildInstallContext(adap, existing, opts)

	// Separate file assets from config assets
	result := &InstallResult{}
	fileAssets, configAssets := i.separateAssets(adap, assets, result)

	// ========================================================================
	// PHASE 1: Resolve all assets and detect conflicts
	// ========================================================================
	resolvedAssets, configByType, conflicts, configConflicts, err := i.resolveAndDetectConflicts(ctx, fileAssets, configAssets)
	if err != nil {
		return nil, err
	}

	// If conflicts exist and not using force/skip flags, return error with all conflicts
	// Note: This applies to dry-run too - we want `--dry-run --diff` to be a natural
	// preview workflow, so dry-run should report conflicts rather than silently ignore them.
	if len(conflicts) > 0 && !opts.Force && !opts.SkipConflicts {
		return nil, &ConflictsError{Conflicts: conflicts}
	}

	// ========================================================================
	// PHASE 2: Install assets
	// ========================================================================
	newEntries, newConfigEntries, err := i.executeInstall(ctx, resolvedAssets, configByType, configConflicts, result)
	if err != nil {
		// Record whatever reached disk before giving up, so a failed install
		// leaves tai's view of the target consistent with the target itself.
		i.saveSalvageManifest(adap, profile, opts, newEntries, newConfigEntries)
		return nil, err
	}

	// Identical warnings carry no extra information the second time, and some
	// are emitted per asset - a target-wide caveat repeated once per hook
	// would bury everything else.
	result.Warnings = dedupeStrings(result.Warnings)

	// Build and save new manifest
	result.Manifest = BuildManifest(adap.Name(), profile, i.configRepo, newEntries, newConfigEntries)
	if !opts.DryRun {
		if err := i.manifestStore.Save(result.Manifest); err != nil {
			return nil, fmt.Errorf("saving manifest: %w", err)
		}
	}

	return result, nil
}

// buildInstallContext creates the shared context for install operations.
func (i *Installer) buildInstallContext(adap adapter.Adapter, existing *Manifest, opts InstallOptions) *installContext {
	existingFiles := make(map[string]ManifestEntry)
	existingConfigEntries := make(map[string]ConfigEntry)
	if existing != nil {
		for _, entry := range existing.Files {
			existingFiles[entry.Path] = entry
		}
		for _, entry := range existing.ConfigEntries {
			existingConfigEntries[entry.Key()] = entry
		}
	}

	mergedVars := template.MergeVariables(adap.ProvideVariables(), opts.Variables)

	var engine *template.Engine
	if i.sources != nil {
		resolver := NewAssetContentResolver(i.sources)
		engine = template.NewEngineWithResolver(mergedVars, resolver)
	} else {
		engine = template.NewEngine(mergedVars)
	}

	untemplated := make(map[string]bool)
	for name, src := range i.sources {
		if !src.Templated() {
			untemplated[name] = true
		}
	}

	return &installContext{
		adap:                  adap,
		targetDir:             adap.TargetDir(),
		templateEngine:        engine,
		untemplated:           untemplated,
		existingFiles:         existingFiles,
		existingConfigEntries: existingConfigEntries,
		configFiles:           newConfigFileSet(),
		consent:               make(map[string]bool),
		opts:                  opts,
	}
}

// engineFor returns the template engine for an asset's content, or nil when its
// source opted out of templating. A file asset's dest is not covered: that is
// written in the user's own config, not in the source.
func (ctx *installContext) engineFor(asset *config.Asset) *template.Engine {
	if ctx.untemplated[asset.SourceName] {
		return nil
	}
	return ctx.templateEngine
}

// separateAssets splits assets into file assets and config assets.
// Unsupported asset types (both config and file) are logged as warnings and skipped.
func (i *Installer) separateAssets(adap adapter.Adapter, assets []*config.Asset, result *InstallResult) ([]*config.Asset, []*config.Asset) {
	var fileAssets, configAssets []*config.Asset

	supportedConfigTypes := make(map[string]bool)
	for _, t := range adap.SupportedConfigAssetTypes() {
		supportedConfigTypes[t] = true
	}

	unsupportedFileTypes := make(map[string]bool)
	for _, t := range adap.UnsupportedFileAssetTypes() {
		unsupportedFileTypes[t] = true
	}

	for _, asset := range assets {
		// Skip unsupported file types before emitting install warnings,
		// since InstallWarnings is intended for assets that will be installed.
		if unsupportedFileTypes[asset.Type] {
			warning := fmt.Sprintf("Asset type '%s' is not supported by target '%s' - skipping %s",
				asset.Type, adap.Name(), asset.Name)
			result.Warnings = append(result.Warnings, warning)
			continue
		}

		result.Warnings = append(result.Warnings, adap.InstallWarnings(asset)...)

		if config.IsConfigAssetType(asset.Type) {
			if !supportedConfigTypes[asset.Type] {
				warning := fmt.Sprintf("Asset type '%s' is not supported by target '%s' - skipping %s",
					asset.Type, adap.Name(), asset.Name)
				result.Warnings = append(result.Warnings, warning)
				continue
			}
			configAssets = append(configAssets, asset)
		} else {
			fileAssets = append(fileAssets, asset)
		}
	}

	return fileAssets, configAssets
}

// resolveAndDetectConflicts is Phase 1 of installation.
// It resolves all assets and detects conflicts upfront.
// Assets are resolved with dryRun=true to prevent backup file creation as a side effect.
func (i *Installer) resolveAndDetectConflicts(
	ctx *installContext,
	fileAssets []*config.Asset,
	configAssets []*config.Asset,
) ([]resolvedFileAsset, map[string]resolvedConfigAsset, []ConflictDetail, map[string]bool, error) {
	var resolvedAssets []resolvedFileAsset
	var conflicts []ConflictDetail

	// Resolve file assets. Distinct assets can map to the same target path - for
	// OpenCode a persona and a delegate of the same name both install to
	// agents/<name>.md, for instance. Collapse such collisions to the last
	// asset, mirroring the "later wins" semantics tai already applies to
	// bundle/source conflicts and the path-keyed manifest dedup in
	// buildInstallContext. Without this, every shadowed asset leaves a second
	// manifest entry whose checksum can never match what's on disk, so reinstall
	// reports a phantom "updated" for it on every run.
	indexByPath := make(map[string]int) // targetPath -> index in resolvedAssets
	var conflictByIndex []*ConflictDetail
	for _, asset := range fileAssets {
		ra, conflict, err := i.resolveFileAsset(ctx, asset)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		if idx, collides := indexByPath[ra.targetPath]; collides {
			prev := resolvedAssets[idx].asset
			i.logger.Printf("Override: %s/%s overrides %s/%s (both install to %s)",
				ra.asset.Type, ra.asset.Name, prev.Type, prev.Name, ra.targetPath)
			resolvedAssets[idx] = ra
			conflictByIndex[idx] = conflict
			continue
		}

		indexByPath[ra.targetPath] = len(resolvedAssets)
		resolvedAssets = append(resolvedAssets, ra)
		conflictByIndex = append(conflictByIndex, conflict)
	}

	for _, conflict := range conflictByIndex {
		if conflict != nil {
			conflicts = append(conflicts, *conflict)
		}
	}

	// Resolve and check config assets. Keying by ConfigEntryKey rather than by
	// type is what lets collection types coexist: two statuslines still
	// collapse to one (a target shows a single status line), while two hooks
	// or two MCP servers keep their own entries.
	configByKey := make(map[string]resolvedConfigAsset)
	configConflicts := make(map[string]bool)

	for _, asset := range configAssets {
		key := ConfigEntryKey(asset.Type, asset.ConfigName())
		if prev, exists := configByKey[key]; exists {
			i.logger.Printf("Override: %s/%s overrides %s/%s",
				asset.Type, asset.Name, prev.asset.Type, prev.asset.Name)
		}
		configByKey[key] = resolvedConfigAsset{asset: asset, scope: ctx.scopeFor(asset)}
	}

	// Both checks read the config files before phase 2 mutates them, which is the
	// only point at which "was tai's entry there?" can still be answered.
	for key, rca := range configByKey {
		if prev, wasManaged := ctx.existingConfigEntries[key]; wasManaged {
			rca.vanished = i.configEntryVanished(ctx, prev)
			configByKey[key] = rca
		}

		conflict := i.checkConfigConflict(ctx, key, rca)
		if conflict != nil {
			conflicts = append(conflicts, *conflict)
			configConflicts[key] = true
		}
	}

	return resolvedAssets, configByKey, conflicts, configConflicts, nil
}

// resolveFileAsset resolves a single file asset and checks for conflicts.
func (i *Installer) resolveFileAsset(ctx *installContext, asset *config.Asset) (resolvedFileAsset, *ConflictDetail, error) {
	// For file assets with templated dest paths, substitute variables
	workingAsset := asset
	if asset.Type == config.AssetTypeFile && asset.Dest != "" && template.HasTemplateSyntax([]byte(asset.Dest)) {
		// {{existing}} doesn't make sense in dest paths - it's for file content only
		if template.HasExistingVar([]byte(asset.Dest)) {
			return resolvedFileAsset{}, nil, fmt.Errorf("{{existing}} cannot be used in dest path for %s (it only works in file content)", asset.Name)
		}
		resolvedDest, err := ctx.templateEngine.ProcessString(asset.Dest)
		if err != nil {
			return resolvedFileAsset{}, nil, fmt.Errorf("processing dest template for %s: %w", asset.Name, err)
		}
		if resolvedDest == "" {
			return resolvedFileAsset{}, nil, fmt.Errorf("dest for %s resolved to empty string; check dest %q for typos or ensure variables have non-empty values", asset.Name, asset.Dest)
		}
		if err := pathutil.ValidateRelativePath(resolvedDest); err != nil {
			return resolvedFileAsset{}, nil, fmt.Errorf("invalid resolved dest for %s: %w", asset.Name, err)
		}
		assetCopy := *asset
		assetCopy.Dest = resolvedDest
		workingAsset = &assetCopy
	}

	targetPath := ctx.adap.MapAssetPath(workingAsset)
	fullTargetPath := filepath.Join(ctx.targetDir, targetPath)

	// Resolve the asset with dryRun=true to prevent backup creation during phase 1
	resolved, err := i.resolveAsset(workingAsset, targetPath, fullTargetPath, ctx.engineFor(asset), ctx.adap, true)
	if err != nil {
		return resolvedFileAsset{}, nil, fmt.Errorf("resolving asset %s: %w", workingAsset.SourcePath, err)
	}
	if warning := fileDestWarning(asset, workingAsset.Dest); warning != "" {
		resolved.Warnings = append(resolved.Warnings, warning)
	}

	existingEntry, wasManaged := ctx.existingFiles[targetPath]
	conflict, existsOnDisk := i.checkConflict(fullTargetPath, existingEntry, wasManaged, resolved)

	ra := resolvedFileAsset{
		asset:         asset,
		workingAsset:  workingAsset,
		targetPath:    targetPath,
		fullPath:      fullTargetPath,
		resolved:      resolved,
		existingEntry: existingEntry,
		wasManaged:    wasManaged,
		existsOnDisk:  existsOnDisk,
		conflict:      conflict,
	}

	var conflictDetail *ConflictDetail
	if conflict != nil {
		var existingContent []byte
		if !resolved.IsDir {
			existingContent, _ = os.ReadFile(fullTargetPath)
		}
		conflictDetail = &ConflictDetail{
			Path:            targetPath,
			FullPath:        fullTargetPath,
			Reason:          conflict.Reason,
			ResolvedContent: resolved.Content,
			ExistingContent: existingContent,
			IsDir:           resolved.IsDir,
		}
	}

	return ra, conflictDetail, nil
}

// executeInstall is Phase 2 of installation.
// It installs the resolved assets and updates the result.
func (i *Installer) executeInstall(
	ctx *installContext,
	resolvedAssets []resolvedFileAsset,
	configByKey map[string]resolvedConfigAsset,
	configConflicts map[string]bool,
	result *InstallResult,
) ([]ManifestEntry, []ConfigEntry, error) {
	var newEntries []ManifestEntry
	var newConfigEntries []ConfigEntry
	opts := ctx.opts

	// Install file assets
	for _, ra := range resolvedAssets {
		entry, skip, err := i.installFileAsset(ctx, ra, result)
		if err != nil {
			return nil, nil, err
		}
		// A conflict skipped via --skip-conflicts stays managed: it keeps its
		// existing manifest entry (stale checksum and all, so it keeps
		// conflicting until resolved) and must come out of the stale set.
		// Dropping it instead made removeStaleFiles treat it as no longer in the
		// profile and delete the very file the user asked to leave alone.
		// Skipped assets tai never managed have no entry to keep.
		if skip && !ra.wasManaged {
			continue
		}

		newEntries = append(newEntries, entry)
		delete(ctx.existingFiles, ra.targetPath)
	}

	// Install config assets. Iterating a map gives no stable order, but the
	// order assets merge in is visible: appended array entries land in it. Sort
	// so repeated installs produce byte-identical config files.
	for _, key := range sortedKeys(configByKey) {
		rca := configByKey[key]
		asset := rca.asset

		if configConflicts[key] {
			if opts.Force {
				// Proceed with overwrite
			} else if opts.SkipConflicts {
				result.Skipped = append(result.Skipped, fmt.Sprintf("[%s] %s", asset.Type, asset.ConfigName()))
				// A skipped entry tai already manages keeps its manifest entry,
				// mirroring how skipped file assets stay managed.
				if prev, wasManaged := ctx.existingConfigEntries[key]; wasManaged {
					newConfigEntries = append(newConfigEntries, prev)
					delete(ctx.existingConfigEntries, key)
				}
				continue
			}
		}

		configEntry, err := i.installConfigAsset(ctx, key, rca, result)
		if err != nil {
			return nil, nil, fmt.Errorf("installing config asset %s/%s: %w", asset.Type, asset.ConfigName(), err)
		}
		if configEntry == nil {
			continue
		}

		label := fmt.Sprintf("[%s] %s", asset.Type, asset.ConfigName())
		existingEntry, wasManaged := ctx.existingConfigEntries[key]
		switch {
		case !wasManaged:
			result.Added = append(result.Added, label)
		// The checksum compares the asset to what the manifest recorded, so an
		// entry edited on disk still matches it. Reporting that as unchanged
		// would hide the fact that a --force just overwrote the edit.
		case configConflicts[key]:
			result.Updated = append(result.Updated, label)
		case rca.vanished:
			result.Restored = append(result.Restored, label)
		case configEntry.Checksum == existingEntry.Checksum:
			result.Unchanged = append(result.Unchanged, label)
		default:
			result.Updated = append(result.Updated, label)
		}

		newConfigEntries = append(newConfigEntries, *configEntry)
		delete(ctx.existingConfigEntries, key)
	}

	// Remove files that are no longer in the profile
	if err := i.removeStaleFiles(ctx, result); err != nil {
		return nil, nil, err
	}

	// Remove config entries that are no longer in the profile
	i.removeStaleConfigEntries(ctx, result)

	// Every config file edit so far has been in memory. Commit them together,
	// after all adds and removals, so each file is written once.
	if !opts.DryRun {
		if _, err := ctx.configFiles.flush(); err != nil {
			// Files are written one at a time, so a failure partway through
			// leaves earlier ones on disk. Hand the entries back anyway: a
			// manifest that records an edit which never landed costs nothing
			// (removal simply finds nothing), while one that misses an edit
			// that did land loses track of it for good - and the next install
			// would append a second copy beside it.
			return newEntries, newConfigEntries, err
		}
	}

	return newEntries, newConfigEntries, nil
}

// saveSalvageManifest persists what an aborted install managed to write.
//
// Without it, entries already merged into a config file would be invisible to
// tai: not in the manifest, so not recognized as its own, so the next install
// appends a second copy and uninstall never cleans them up. Best effort by
// design - the install is already failing, and the original error is the one
// worth reporting.
func (i *Installer) saveSalvageManifest(
	adap adapter.Adapter,
	profile string,
	opts InstallOptions,
	entries []ManifestEntry,
	configEntries []ConfigEntry,
) {
	if opts.DryRun || (len(entries) == 0 && len(configEntries) == 0) {
		return
	}

	manifest := BuildManifest(adap.Name(), profile, i.configRepo, entries, configEntries)
	if err := i.manifestStore.Save(manifest); err != nil {
		i.logger.Printf(
			"Install failed and the partial manifest could not be saved either (%v). "+
				"Entries already written to the target's config files are not tracked by tai: "+
				"check them by hand, or re-run the install once the underlying problem is fixed.",
			err)
	}
}

// dedupeStrings drops repeated entries, keeping first-seen order.
func dedupeStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := in[:0:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// sortedKeys returns a map's keys in a deterministic order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// installFileAsset installs a single resolved file asset.
// Returns the manifest entry, whether to skip (for conflicts), and any error.
func (i *Installer) installFileAsset(ctx *installContext, ra resolvedFileAsset, result *InstallResult) (ManifestEntry, bool, error) {
	opts := ctx.opts

	if ra.conflict != nil {
		if opts.Force {
			// Proceed with overwrite
		} else if opts.SkipConflicts {
			result.Skipped = append(result.Skipped, ra.targetPath)
			if ra.wasManaged {
				return ra.existingEntry, true, nil
			}
			return ManifestEntry{}, true, nil
		}
	}

	// Create backup for {{existing}} assets if not dry-run
	// (Phase 1 skipped backup creation by using dryRun=true)
	if !opts.DryRun && ra.resolved.BackupPath != "" {
		if _, err := os.Stat(ra.fullPath); os.IsNotExist(err) {
			return ManifestEntry{}, false, fmt.Errorf("cannot create backup for %s: file was removed after conflict check", ra.targetPath)
		}
		if err := pathutil.CopyFile(ra.fullPath, ra.resolved.BackupPath); err != nil {
			return ManifestEntry{}, false, fmt.Errorf("creating backup for %s: %w", ra.targetPath, err)
		}
	}

	if !opts.DryRun {
		if err := i.writeFileAsset(ctx, ra.resolved, ra.fullPath); err != nil {
			return ManifestEntry{}, false, fmt.Errorf("installing %s: %w", ra.targetPath, err)
		}
	} else if ra.resolved.IsDir {
		if err := i.previewDirAsset(ctx, ra.resolved); err != nil {
			return ManifestEntry{}, false, fmt.Errorf("previewing %s: %w", ra.targetPath, err)
		}
	}

	// Collected after the install, not before: a directory asset's main file is
	// only parsed once the directory has been copied and its includes resolved,
	// so its frontmatter warnings do not exist until then.
	result.Warnings = append(result.Warnings, ra.resolved.Warnings...)

	// Track what's happening. Absence outranks the checksum comparison: a file
	// that was gone and is back has changed, and the manifest checksum it would
	// otherwise be judged against describes a copy nobody can see any more.
	switch {
	case !ra.wasManaged:
		result.Added = append(result.Added, ra.targetPath)
	case !ra.existsOnDisk:
		result.Restored = append(result.Restored, ra.targetPath)
	case ra.resolved.Checksum == ra.existingEntry.Checksum:
		result.Unchanged = append(result.Unchanged, ra.targetPath)
	default:
		result.Updated = append(result.Updated, ra.targetPath)
	}

	return EntryFromAsset(ra.resolved), false, nil
}

// writeFileAsset installs a resolved asset at path. A directory's checksum is
// then retaken from what was written, since includes and the adapter rewrite
// its contents on the way in and the manifest must describe the installed copy.
func (i *Installer) writeFileAsset(ctx *installContext, resolved *config.ResolvedAsset, path string) error {
	if err := i.installAsset(resolved, path, ctx.adap, ctx.engineFor(&resolved.Asset)); err != nil {
		return err
	}
	if !resolved.IsDir {
		return nil
	}
	checksum, err := ComputeDirChecksum(path)
	if err != nil {
		return fmt.Errorf("computing installed checksum: %w", err)
	}
	resolved.Checksum = checksum
	return nil
}

// previewDirAsset builds a directory asset in a scratch directory, for a dry
// run. Its installed checksum and its main file's frontmatter warnings only
// exist once it has been built; judged by the source checksum instead, every
// directory would read as updated and none would warn.
func (i *Installer) previewDirAsset(ctx *installContext, resolved *config.ResolvedAsset) error {
	scratch, err := os.MkdirTemp("", "tai-dry-run-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	return i.writeFileAsset(ctx, resolved, filepath.Join(scratch, filepath.Base(resolved.TargetPath)))
}

// removeStaleFiles removes files that are no longer in the profile.
func (i *Installer) removeStaleFiles(ctx *installContext, result *InstallResult) error {
	for path, entry := range ctx.existingFiles {
		fullPath := filepath.Join(ctx.targetDir, path)
		result.Removed = append(result.Removed, path)

		if !ctx.opts.DryRun {
			if entry.BackupPath != "" {
				if err := i.restoreBackup(entry.BackupPath, fullPath); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("restoring backup for %s: %w", path, err))
				}
			} else {
				if entry.IsDir {
					if err := os.RemoveAll(fullPath); err != nil && !os.IsNotExist(err) {
						result.Errors = append(result.Errors, fmt.Errorf("removing %s: %w", path, err))
					}
				} else {
					if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
						result.Errors = append(result.Errors, fmt.Errorf("removing %s: %w", path, err))
					}
				}
				cleanEmptyDirs(fullPath, ctx.targetDir)
			}
		}
	}
	return nil
}

// removeStaleConfigEntries removes config entries that are no longer in the profile.
func (i *Installer) removeStaleConfigEntries(ctx *installContext, result *InstallResult) {
	for _, key := range sortedKeys(ctx.existingConfigEntries) {
		entry := ctx.existingConfigEntries[key]
		result.Removed = append(result.Removed, fmt.Sprintf("[%s] %s", entry.Type, entry.Name))
		if ctx.opts.DryRun {
			continue
		}
		if err := i.revertConfigEdits(ctx, entry); err != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("removing config %s/%s: %w", entry.Type, entry.Name, err))
		}
	}
}

// checkConfigConflict reports whether installing a config asset would collide
// with an entry tai does not manage.
//
// Conflict is judged per entry, not per settings key. Judging by key would be
// useless for shared keys: every Claude user has a "hooks" object holding
// entries from other tools, so "the key exists" would flag every install while
// telling nobody anything. What matters is whether the specific entry tai is
// about to write is already there under someone else's ownership.
func (i *Installer) checkConfigConflict(ctx *installContext, key string, rca resolvedConfigAsset) *ConflictDetail {
	if prev, wasManaged := ctx.existingConfigEntries[key]; wasManaged {
		// tai wrote this entry, but that only makes it tai's if it still looks
		// the way tai left it. An entry edited since is the user's now, and
		// reinstalling over it would silently discard their change - the same
		// drift a modified file asset is gated on.
		if where, drifted := i.configEntryDrifted(ctx, prev); drifted {
			return &ConflictDetail{
				Path:   fmt.Sprintf("[%s] %s", rca.asset.Type, rca.asset.ConfigName()),
				Reason: fmt.Sprintf("%s modified since last install", where),
			}
		}
		return nil // Still tai's; an update, not a conflict.
	}

	plan, err := i.planConfigAsset(ctx, rca)
	if err != nil {
		// Planning problems surface with full context at install time; here we
		// only decline to claim a conflict we cannot actually check for.
		return nil
	}

	for _, edit := range plan.Edits {
		doc, err := ctx.configFiles.load(edit.File)
		if err != nil {
			return nil // Unreadable config file; install will report it properly.
		}
		if occupied, where := editCollides(doc, edit); occupied {
			return &ConflictDetail{
				Path:   fmt.Sprintf("[%s] %s", rca.asset.Type, rca.asset.ConfigName()),
				Reason: fmt.Sprintf("unmanaged %s exists in %s", where, edit.File),
			}
		}
	}

	return nil
}

// configEntryDrifted reports whether any entry tai previously wrote no longer
// matches what it recorded, naming the first spot that differs.
//
// An entry that vanished is not drift: the user removing it is not a change to
// preserve, and treating it as a conflict would make a reinstall that simply
// puts it back require --force.
func (i *Installer) configEntryDrifted(ctx *installContext, entry ConfigEntry) (string, bool) {
	for _, applied := range entry.Edits {
		doc, err := ctx.configFiles.load(applied.File)
		if err != nil {
			return "", false // Unreadable; install will report it properly.
		}
		current, exists := findEntry(doc, applied)
		if !exists {
			continue
		}
		if !checksumMatches(current, applied.Checksum) {
			return fmt.Sprintf("%s in %s", pathString(applied.KeyPath), applied.File), true
		}
	}
	return "", false
}

// configEntryVanished reports whether any edit tai previously wrote is no
// longer in the config file - the case configEntryDrifted deliberately passes
// over, since putting an entry back is not a conflict.
//
// Not a conflict, but still worth reporting: an install that re-adds a hook
// someone deleted from settings.json has changed that file, and a summary
// calling it unchanged is the same lie as one calling a restored file
// unchanged.
func (i *Installer) configEntryVanished(ctx *installContext, entry ConfigEntry) bool {
	for _, applied := range entry.Edits {
		doc, err := ctx.configFiles.load(applied.File)
		if err != nil {
			return false // Unreadable; install will report it properly.
		}
		if _, exists := findEntry(doc, applied); !exists {
			return true
		}
	}
	return false
}

// editCollides reports whether an edit would displace an entry tai does not
// own, and names the spot for the conflict message.
//
// Only modes that overwrite can collide. AppendToArray adds alongside whatever
// is already there, so it never displaces anything and never conflicts - which
// is exactly what makes hooks from several tools able to share one event.
func editCollides(doc map[string]any, edit configasset.Edit) (bool, string) {
	parent, key, err := descend(doc, edit.KeyPath, false)
	if err != nil || parent == nil {
		return false, ""
	}

	switch edit.Mode {
	case configasset.ReplaceKey:
		if _, exists := parent[key]; exists {
			return true, key
		}
	case configasset.UpsertMapEntry:
		container, ok := parent[key].(map[string]any)
		if !ok {
			return false, ""
		}
		if _, exists := container[edit.ID]; exists {
			return true, fmt.Sprintf("%s.%s", key, edit.ID)
		}
	}

	return false, ""
}

// planConfigAsset reads, templates, and plans a config asset without writing
// anything. Used by both the conflict check and the install itself.
func (i *Installer) planConfigAsset(ctx *installContext, rca resolvedConfigAsset) (*configasset.Plan, error) {
	handler, ok := configasset.Lookup(rca.asset.Type)
	if !ok {
		return nil, fmt.Errorf("unsupported config asset type: %s", rca.asset.Type)
	}

	content, err := i.configAssetContent(ctx, rca.asset)
	if err != nil {
		return nil, err
	}

	return handler.Plan(configasset.Request{
		Adapter: ctx.adap,
		Asset:   rca.asset,
		Content: content,
		Scope:   rca.scope,
		Name:    rca.asset.ConfigName(),
	})
}

// configAssetContent reads a config asset and runs it through the template
// engine, so config assets get the same variables and conditionals as file
// assets - a hook command can reference {{ target_agents_file }}, an MCP env
// block can reference a var.
func (i *Installer) configAssetContent(ctx *installContext, asset *config.Asset) ([]byte, error) {
	data, err := os.ReadFile(asset.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", asset.SourcePath, err)
	}

	// No target path: {{ existing }} has no meaning for a config asset, whose
	// output is entries merged into a shared document rather than a file whose
	// prior content could be folded in.
	engine := ctx.engineFor(asset)
	if engine == nil {
		return data, nil
	}
	rendered, err := engine.Process(data, "")
	if err != nil {
		return nil, fmt.Errorf("templating %s: %w", asset.Name, err)
	}

	return rendered, nil
}

// installConfigAsset merges a config asset's entries into the target's config
// files and returns the manifest record needed to remove them again.
//
// Nothing is written until the install is known to be going ahead. Only then
// are previously-applied edits for this same entry reversed, so an update
// replaces tai's entry rather than appending a second copy of it, and
// reinstalling an unchanged asset is a no-op.
func (i *Installer) installConfigAsset(
	ctx *installContext,
	key string,
	rca resolvedConfigAsset,
	result *InstallResult,
) (*ConfigEntry, error) {
	plan, err := i.planConfigAsset(ctx, rca)
	if err != nil {
		return nil, err
	}
	result.Warnings = append(result.Warnings, plan.Warnings...)

	prev, wasManaged := ctx.existingConfigEntries[key]

	// Consent is resolved before anything is touched. Declining means "do not
	// write this file", so an entry already installed under an earlier yes stays
	// exactly as it is - returning it unchanged keeps it in the manifest, where
	// removing it would strand config tai still owns.
	if plan.Consent != nil {
		granted, err := i.askConsent(ctx, *plan.Consent)
		if err != nil {
			return nil, err
		}
		if !granted {
			result.Warnings = append(result.Warnings,
				noConsentWarning(ctx, rca, plan, wasManaged))
			if wasManaged {
				return &prev, nil
			}
			return nil, nil
		}
	}

	// A plan can legitimately produce nothing - a hook whose event this target
	// does not have, for instance. Record no manifest entry for it: an entry
	// with no edits is indistinguishable from one written before tai tracked
	// edits, and would be removed by deleting a whole settings key. Leaving any
	// previous entry in place hands it to the stale-entry sweep, which removes
	// it properly and reports it.
	if len(plan.Edits) == 0 {
		return nil, nil
	}

	// Each new edit is matched against the one it supersedes so it can be
	// rewritten where it already sits. Whatever is left over is an edit this
	// install no longer makes - a hook that changed event, say - and is removed.
	superseded := supersededEdits(prev, plan.Edits)
	if wasManaged {
		// An entry with no edits at all is a legacy one, removed by deleting its
		// whole settings key; hand that case through untouched. Otherwise revert
		// only what this install is not about to rewrite, and skip the call
		// entirely when that leaves nothing - an empty edit list would be
		// mistaken for legacy.
		stale := prev.withoutEdits(superseded)
		if len(prev.Edits) == 0 || len(stale.Edits) > 0 {
			if err := i.revertConfigEdits(ctx, stale); err != nil {
				return nil, err
			}
		}
	}

	applied := make([]configasset.AppliedEdit, 0, len(plan.Edits))
	for _, edit := range plan.Edits {
		doc, err := ctx.configFiles.load(edit.File)
		if err != nil {
			return nil, err
		}

		record, updated, err := updateSupersededEdit(doc, superseded, edit, ctx.opts.Force)
		if err != nil {
			return nil, fmt.Errorf("merging %s/%s into %s: %w",
				rca.asset.Type, rca.asset.ConfigName(), edit.File, err)
		}
		if !updated {
			record, err = applyEdit(doc, edit)
			if err != nil {
				return nil, fmt.Errorf("merging %s/%s into %s: %w",
					rca.asset.Type, rca.asset.ConfigName(), edit.File, err)
			}
		}
		ctx.configFiles.markDirty(edit.File)
		applied = append(applied, record)
	}

	entry := &ConfigEntry{
		Type:         rca.asset.Type,
		Name:         rca.asset.ConfigName(),
		SourceName:   rca.asset.SourceName,
		SourcePath:   rca.asset.SourcePath,
		SettingsFile: primaryFile(applied),
		Scope:        rca.scope,
		Checksum:     editsChecksum(applied),
		Edits:        applied,
	}

	return entry, nil
}

// askConsent resolves a consent request once per install, caching the answer
// so a profile with several MCP servers asks about the file once rather than
// once per server.
func (i *Installer) askConsent(ctx *installContext, req configasset.ConsentRequest) (bool, error) {
	if granted, asked := ctx.consent[req.Key]; asked {
		return granted, nil
	}

	// A preview must not decide anything. Asking here would put a real, durable
	// answer - possibly a remembered one - behind a command whose whole promise
	// is that it changes nothing, and a later real install would silently reuse
	// it. Report what would be asked instead.
	if ctx.opts.DryRun {
		ctx.consent[req.Key] = false
		return false, nil
	}

	if ctx.opts.Consent == nil {
		ctx.consent[req.Key] = false
		return false, nil
	}

	granted, err := ctx.opts.Consent(req)
	if err != nil {
		return false, err
	}
	ctx.consent[req.Key] = granted
	return granted, nil
}

// editSlot identifies the place in a config file an edit occupies, so a new
// edit can be matched to the one it replaces.
type editSlot struct {
	file    string
	keyPath string
	mode    configasset.MergeMode
	id      string
}

func slotOf(file string, keyPath []string, mode configasset.MergeMode, id string) editSlot {
	return editSlot{file: file, keyPath: pathString(keyPath), mode: mode, id: id}
}

// supersededEdits pairs each new edit with the applied edit it replaces.
func supersededEdits(prev ConfigEntry, edits []configasset.Edit) map[editSlot]configasset.AppliedEdit {
	wanted := make(map[editSlot]bool, len(edits))
	for _, e := range edits {
		wanted[slotOf(e.File, e.KeyPath, e.Mode, e.ID)] = true
	}

	matched := make(map[editSlot]configasset.AppliedEdit)
	for _, a := range prev.Edits {
		slot := slotOf(a.File, a.KeyPath, a.Mode, a.ID)
		if wanted[slot] {
			matched[slot] = a
		}
	}
	return matched
}

// updateSupersededEdit rewrites an edit in the place its predecessor occupies,
// reporting whether it did.
func updateSupersededEdit(
	doc map[string]any,
	superseded map[editSlot]configasset.AppliedEdit,
	e configasset.Edit,
	force bool,
) (configasset.AppliedEdit, bool, error) {
	prev, ok := superseded[slotOf(e.File, e.KeyPath, e.Mode, e.ID)]
	if !ok {
		return configasset.AppliedEdit{}, false, nil
	}
	return updateEdit(doc, prev, e, force)
}

// noConsentWarning explains a config write that did not go ahead: what was
// skipped, what that means for the target, and how to allow it.
func noConsentWarning(ctx *installContext, rca resolvedConfigAsset, plan *configasset.Plan, wasManaged bool) string {
	label := fmt.Sprintf("[%s] %s", rca.asset.Type, rca.asset.ConfigName())
	file := primaryPlanFile(plan)

	if ctx.opts.DryRun {
		return fmt.Sprintf(
			"Skipped %s: writing %s needs your say-so, which a dry run will not ask for. "+
				"Run without --dry-run to decide, or pass --allow-config-rewrite.",
			label, file)
	}

	outcome := fmt.Sprintf("it will not be available in %s", ctx.adap.Name())
	if wasManaged {
		outcome = "the copy from a previous install is left untouched"
	}

	return fmt.Sprintf(
		"Skipped %s: writing %s was declined, so %s. "+
			"To allow it, re-run with --allow-config-rewrite, or answer the prompt in an interactive terminal.",
		label, file, outcome)
}

// primaryPlanFile names the file a plan would write, for messages.
func primaryPlanFile(plan *configasset.Plan) string {
	if len(plan.Edits) == 0 {
		return "the target config"
	}
	return plan.Edits[0].File
}

// revertConfigEdits removes the entries a previous install wrote.
func (i *Installer) revertConfigEdits(ctx *installContext, entry ConfigEntry) error {
	// Entries recorded before tai tracked edits carry no Edits. They were
	// installed by replacing a whole settings key, so that is how they come
	// back out.
	//
	// Restricted to singleton types on purpose. Collection types postdate edit
	// tracking, so an empty Edits list can only mean something else - and
	// deleting their settings key would take out every other tool's entries
	// along with tai's.
	if len(entry.Edits) == 0 {
		if configasset.IsCollection(entry.Type) {
			return nil
		}
		return i.revertLegacyConfigEntry(ctx, entry)
	}

	for _, applied := range entry.Edits {
		doc, err := ctx.configFiles.load(applied.File)
		if err != nil {
			return err
		}

		changed, found, err := removeEdit(doc, applied)
		if err != nil {
			return err
		}
		if changed {
			ctx.configFiles.markDirty(applied.File)
		}
		if !found {
			i.logger.Printf(
				"Config entry %s/%s in %s was modified or removed outside tai - leaving it in place",
				entry.Type, entry.Name, applied.File)
		}
	}

	return nil
}

// revertLegacyConfigEntry removes a config entry installed before edit
// tracking existed, by deleting the settings key that carried it.
func (i *Installer) revertLegacyConfigEntry(ctx *installContext, entry ConfigEntry) error {
	settingsKey := ctx.adap.SettingsKey(entry.Type)
	if settingsKey == "" || entry.SettingsFile == "" {
		return nil
	}

	doc, err := ctx.configFiles.load(entry.SettingsFile)
	if err != nil {
		return err
	}
	if _, exists := doc[settingsKey]; !exists {
		return nil
	}

	delete(doc, settingsKey)
	ctx.configFiles.markDirty(entry.SettingsFile)
	return nil
}

// primaryFile names the config file a manifest entry is reported against.
func primaryFile(applied []configasset.AppliedEdit) string {
	if len(applied) == 0 {
		return ""
	}
	return applied[0].File
}

// editsChecksum summarizes every edit an asset produced, so an unchanged
// reinstall is recognizable as unchanged.
func editsChecksum(applied []configasset.AppliedEdit) string {
	combined := ""
	for _, a := range applied {
		combined += a.File + "\x00" + pathString(a.KeyPath) + "\x00" + a.ID + "\x00" + a.Checksum + "\n"
	}
	return ComputeChecksum([]byte(combined))
}

// Uninstall removes all tai-managed files for a target.
func (i *Installer) Uninstall(adap adapter.Adapter) (*InstallResult, error) {
	targetDir := adap.TargetDir()
	result := &InstallResult{}

	manifest, err := i.manifestStore.Load(adap.Name())
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}
	if manifest == nil {
		return result, nil // Nothing installed
	}

	// Remove file entries
	for _, entry := range manifest.Files {
		fullPath := filepath.Join(targetDir, entry.Path)
		result.Removed = append(result.Removed, entry.Path)

		// Restore backup if exists
		if entry.BackupPath != "" {
			if err := i.restoreBackup(entry.BackupPath, fullPath); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("restoring backup for %s: %w", entry.Path, err))
			}
		} else {
			// Remove the file/directory
			if entry.IsDir {
				if err := os.RemoveAll(fullPath); err != nil && !os.IsNotExist(err) {
					result.Errors = append(result.Errors, fmt.Errorf("removing %s: %w", entry.Path, err))
				}
			} else {
				if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
					result.Errors = append(result.Errors, fmt.Errorf("removing %s: %w", entry.Path, err))
				}
			}
			// Clean up empty parent directories
			cleanEmptyDirs(fullPath, targetDir)
		}
	}

	// Remove config entries, taking back exactly the entries tai wrote and
	// leaving entries from the user and other tools untouched.
	uninstallCtx := &installContext{
		adap:        adap,
		configFiles: newConfigFileSet(),
		consent:     make(map[string]bool),
	}
	for _, entry := range manifest.ConfigEntries {
		result.Removed = append(result.Removed, fmt.Sprintf("[%s] %s", entry.Type, entry.Name))
		if err := i.revertConfigEdits(uninstallCtx, entry); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("removing config %s/%s: %w", entry.Type, entry.Name, err))
		}
	}
	if _, err := uninstallCtx.configFiles.flush(); err != nil {
		result.Errors = append(result.Errors, err)
	}

	// Delete manifest
	if err := i.manifestStore.Delete(adap.Name()); err != nil {
		result.Errors = append(result.Errors, err)
	}

	return result, nil
}

// resolveAsset resolves an asset to a ResolvedAsset with content loaded.
// If dryRun is true, backup paths are computed but backups are not created.
func (i *Installer) resolveAsset(
	asset *config.Asset,
	targetPath, fullTargetPath string,
	engine *template.Engine,
	adap adapter.Adapter,
	dryRun bool,
) (*config.ResolvedAsset, error) {
	resolved := &config.ResolvedAsset{
		Asset:      *asset,
		TargetPath: targetPath,
	}

	if asset.IsDir {
		// Directory asset - compute directory checksum
		checksum, err := ComputeDirChecksum(asset.SourcePath)
		if err != nil {
			return nil, err
		}
		resolved.Checksum = checksum

		// Validate templates in source files early (Phase 1) to catch
		// errors before any files are copied to the target directory.
		if engine != nil {
			if err := validateDirTemplates(asset.SourcePath, engine); err != nil {
				return nil, fmt.Errorf("validating templates in %s/%s: %w", asset.Type, asset.Name, err)
			}
		}
	} else {
		// File asset - read content and capture permissions
		srcInfo, err := os.Stat(asset.SourcePath)
		if err != nil {
			return nil, err
		}
		resolved.FileMode = srcInfo.Mode().Perm()

		content, err := os.ReadFile(asset.SourcePath)
		if err != nil {
			return nil, err
		}

		if engine != nil && template.HasTemplateSyntax(content) {
			resolved.IsTemplated = true

			// If uses {{existing}}, prepare backup of the existing file
			if template.HasExistingVar(content) {
				if _, err := os.Stat(fullTargetPath); err == nil {
					backupPath := fullTargetPath + ".tai-backup"
					// Only create actual backup if not dry-run
					if !dryRun {
						if err := pathutil.CopyFile(fullTargetPath, backupPath); err != nil {
							return nil, fmt.Errorf("creating backup: %w", err)
						}
					}
					resolved.BackupPath = backupPath
				}
			}

			// Apply templating
			processed, err := engine.Process(content, fullTargetPath)
			if err != nil {
				return nil, fmt.Errorf("processing template: %w", err)
			}
			content = processed
		}

		// File type assets skip frontmatter processing - they're copied as-is
		if asset.Type == config.AssetTypeFile {
			resolved.Content = content
			resolved.Checksum = ComputeChecksum(resolved.Content)
			return resolved, nil
		}

		// Parse and validate frontmatter for all other file asset types
		parsed, err := frontmatter.Parse(content)
		if err != nil {
			return nil, fmt.Errorf("parsing frontmatter: %w", err)
		}

		i.claimNativeFrontmatter(parsed, asset.SourceName)

		if err := validateAssetMetadata(parsed.Metadata, asset.Type); err != nil {
			return nil, fmt.Errorf("asset %s/%s: %w", asset.Type, asset.Name, err)
		}

		resolved.Metadata = parsed.Metadata
		resolved.Native = parsed.Native
		resolved.Content = parsed.Body
		resolved.Warnings = append(resolved.Warnings,
			frontmatterWarnings(parsed, asset.Type, asset.Name, adap.Name(), adapter.TargetNames())...)

		// Let the adapter transform content (e.g., add target-specific frontmatter)
		transformed, err := adap.TransformContent(resolved)
		if err != nil {
			return nil, fmt.Errorf("transforming content: %w", err)
		}
		resolved.Content = transformed

		resolved.Checksum = ComputeChecksum(resolved.Content)
	}

	return resolved, nil
}

// checkConflict checks for conflicts when installing a file, and reports
// whether the target is currently on disk. Both answers come from one stat, and
// the caller needs the second to tell a restore apart from a no-op.
func (i *Installer) checkConflict(
	fullPath string,
	existingEntry ManifestEntry,
	wasManaged bool,
	resolved *config.ResolvedAsset,
) (*Conflict, bool) {
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return nil, false // No conflict - file doesn't exist
	}
	if err != nil {
		// Something is there that tai cannot read. Call it existing, so a stat
		// failure cannot pass itself off as a restore.
		return &Conflict{Path: fullPath, Reason: fmt.Sprintf("stat error: %v", err)}, true
	}

	if !wasManaged {
		// File exists but wasn't managed by tai
		// However, if the asset uses {{existing}}, it's designed to wrap existing content
		if resolved.BackupPath != "" {
			return nil, true // Not a conflict - we're wrapping existing content
		}
		return &Conflict{Path: fullPath, Reason: "unmanaged file exists"}, true
	}

	// Check if user modified the file
	var currentChecksum string
	if info.IsDir() {
		currentChecksum, _ = ComputeDirChecksum(fullPath)
	} else {
		currentChecksum, _ = ComputeFileChecksum(fullPath)
	}

	if currentChecksum != existingEntry.Checksum {
		return &Conflict{Path: fullPath, Reason: "file modified since last install"}, true
	}

	return nil, true
}

// installAsset installs a resolved asset to the target.
func (i *Installer) installAsset(resolved *config.ResolvedAsset, fullTargetPath string, adap adapter.Adapter, engine *template.Engine) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullTargetPath), 0755); err != nil {
		return err
	}

	if resolved.IsDir {
		return i.installDirAsset(resolved, fullTargetPath, adap, engine)
	}

	// Write file content, preserving source permissions
	mode := resolved.FileMode
	if mode == 0 {
		mode = 0644
	}
	if err := os.WriteFile(fullTargetPath, resolved.Content, mode); err != nil {
		return err
	}
	// WriteFile only applies mode on creation; explicitly set it for updates
	return os.Chmod(fullTargetPath, mode)
}

// installDirAsset installs a directory asset (persona, delegate, skill, etc.).
func (i *Installer) installDirAsset(resolved *config.ResolvedAsset, fullTargetPath string, adap adapter.Adapter, engine *template.Engine) error {
	mainFileName := adap.MainFileName(resolved.Type)
	if mainFileName == "" {
		return fmt.Errorf("directory assets not supported for type %q", resolved.Type)
	}

	// For skills, we need special handling:
	// 1. Copy directory INCLUDING the main file (but excluding tai.yaml)
	// 2. Resolve includes (which may replace the main file)
	// 3. Transform the main file from the installed location
	if resolved.Type == config.AssetTypeSkill {
		return i.installSkillDirAsset(resolved, fullTargetPath, adap, mainFileName, engine)
	}

	// For other directory types (persona, delegate):
	// 1. Copy directory contents, excluding the main file
	// 2. Copy and transform the main file from source
	if err := copyDirExcluding(resolved.SourcePath, fullTargetPath, mainFileName); err != nil {
		return err
	}

	// Clean up the target directory if any subsequent step fails, to avoid
	// leaving an orphaned unmanaged directory that blocks future installs.
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(fullTargetPath)
		}
	}()

	// Template-process non-main files in the copied directory
	if err := templateProcessDirFiles(fullTargetPath, engine, func(rel string) bool { return rel == mainFileName }); err != nil {
		return err
	}

	// Transform the main file (read from source, write to target)
	if err := i.transformMainFileFromSource(resolved, fullTargetPath, adap, mainFileName, engine); err != nil {
		return err
	}

	succeeded = true
	return nil
}

// installSkillDirAsset handles the special case of skill directories.
// Skills need to copy the directory first, resolve includes (which may replace files),
// then transform the main file from its installed location.
func (i *Installer) installSkillDirAsset(resolved *config.ResolvedAsset, fullTargetPath string, adap adapter.Adapter, mainFileName string, engine *template.Engine) error {
	// Copy directory excluding only the manifest file (main file gets copied)
	if err := copyDirExcluding(resolved.SourcePath, fullTargetPath, SkillManifestFilename); err != nil {
		return err
	}

	// Clean up the target directory if any subsequent step fails, to avoid
	// leaving an orphaned unmanaged directory that blocks future installs.
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.RemoveAll(fullTargetPath)
		}
	}()

	// Load and process skill manifest if present
	manifest, err := config.LoadSkillManifest(resolved.SourcePath)
	if err != nil {
		return fmt.Errorf("loading skill manifest: %w", err)
	}
	var writes []IncludeWrite
	if manifest != nil && len(manifest.Include) > 0 {
		resolver := NewIncludeResolver(i.sources, i.httpClient)
		resolver.SetCommandContext(CommandContext{
			AssetType:   config.AssetTypeSkill,
			AssetName:   resolved.Name,
			TargetName:  adap.Name(),
			ManifestDir: resolved.SourcePath,
			Version:     i.version,
		})
		writes, err = resolver.ResolveIncludes(manifest, fullTargetPath)
		if err != nil {
			return fmt.Errorf("resolving includes: %w", err)
		}
	}

	// An included file keeps its source's settings, so each file is judged
	// by the include that wrote it last, falling back to the skill's own
	// source. Command and URL includes belong to no source.
	sourceOf := func(rel string) string {
		for k := len(writes) - 1; k >= 0; k-- {
			if isExcludedPath(rel, []string{writes[k].Path}) {
				return writes[k].From
			}
		}
		return resolved.SourceName
	}
	untemplated := func(rel string) bool {
		src, ok := i.sources[sourceOf(rel)]
		return ok && !src.Templated()
	}

	// Template-process non-main files in the copied directory
	if err := templateProcessDirFiles(fullTargetPath, engine, func(rel string) bool {
		return rel == mainFileName || untemplated(rel)
	}); err != nil {
		return err
	}
	if untemplated(mainFileName) {
		engine = nil
	}

	// Detect field mapping for SKILL.md from includes
	fieldMapping := findSkillMdFields(manifest)

	// SKILL.md's frontmatter was written for whichever source supplied it.
	mainSource := sourceOf(mainFileName)

	// Transform SKILL.md from installed location (after includes may have replaced it)
	if err := i.transformMainFileInPlace(resolved, fullTargetPath, adap, mainFileName, fieldMapping, engine, mainSource); err != nil {
		return err
	}

	succeeded = true
	return nil
}

// transformMainFileFromSource reads a main file from the source directory,
// transforms it, and writes to the target directory.
// Used for persona and delegate directory assets.
func (i *Installer) transformMainFileFromSource(resolved *config.ResolvedAsset, dirTargetPath string, adap adapter.Adapter, mainFileName string, engine *template.Engine) error {
	srcMainFile := filepath.Join(resolved.SourcePath, mainFileName)
	dstMainFile := filepath.Join(dirTargetPath, mainFileName)

	srcInfo, err := os.Stat(srcMainFile)
	if os.IsNotExist(err) {
		return fmt.Errorf("main file %s not found in %s directory %q", mainFileName, resolved.Type, resolved.Name)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", mainFileName, err)
	}
	mode := srcInfo.Mode().Perm()

	content, err := os.ReadFile(srcMainFile)
	if err != nil {
		return fmt.Errorf("reading %s: %w", mainFileName, err)
	}

	transformed, err := i.transformMainFileContent(resolved, content, mainFileName, nil, adap, engine, resolved.SourceName)
	if err != nil {
		return err
	}

	return os.WriteFile(dstMainFile, transformed, mode)
}

// transformMainFileInPlace reads a main file from the target directory (after
// includes may have copied/replaced it), transforms it, and writes it back.
// Used for skill directory assets.
//
// If fieldMapping is non-nil, it remaps source field names to tai field names
// before validation - e.g., {"name": "title"} renames the source's "title"
// field to tai's "name" field.
func (i *Installer) transformMainFileInPlace(resolved *config.ResolvedAsset, dirTargetPath string, adap adapter.Adapter, mainFileName string, fieldMapping map[string]string, engine *template.Engine, contentSource string) error {
	mainFilePath := filepath.Join(dirTargetPath, mainFileName)

	info, err := os.Stat(mainFilePath)
	if os.IsNotExist(err) {
		return nil // No main file — nothing to transform
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", mainFileName, err)
	}
	mode := info.Mode().Perm()

	content, err := os.ReadFile(mainFilePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", mainFileName, err)
	}

	transformed, err := i.transformMainFileContent(resolved, content, mainFileName, fieldMapping, adap, engine, contentSource)
	if err != nil {
		return err
	}

	return os.WriteFile(mainFilePath, transformed, mode)
}

// transformMainFileContent parses, validates, and transforms main file content.
// This is the shared logic used by both transformMainFileFromSource and transformMainFileInPlace.
//
// Warnings about the main file's frontmatter are recorded on resolved, which
// the installer drains once the asset is installed.
//
// contentSource is the source the main file came from, whose
// native_frontmatter decides what its unknown fields are.
func (i *Installer) transformMainFileContent(resolved *config.ResolvedAsset, content []byte, mainFileName string, fieldMapping map[string]string, adap adapter.Adapter, engine *template.Engine, contentSource string) ([]byte, error) {
	assetType, assetName := resolved.Type, resolved.Name

	// Apply template processing before frontmatter parsing.
	// {{ existing }} is not supported in directory assets - there's no
	// single target file to read existing content from.
	if engine != nil && template.HasTemplateSyntax(content) {
		if template.HasExistingVar(content) {
			return nil, fmt.Errorf("%s: {{ existing }} is not supported in directory assets", mainFileName)
		}
		processed, err := engine.Process(content, "")
		if err != nil {
			return nil, fmt.Errorf("processing template in %s: %w", mainFileName, err)
		}
		content = processed
	}

	parsed, err := frontmatter.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parsing %s frontmatter: %w", mainFileName, err)
	}

	// Apply field mapping: renames source field names to tai field names.
	if len(fieldMapping) > 0 {
		if parsed.Metadata == nil {
			parsed.Metadata = make(map[string]string)
		}
		// Snapshot source values before mutating. A list is joined with
		// commas: the field being mapped is usually Agent Skills'
		// allowed-tools, often a YAML list, mapped onto tai's comma-separated
		// tools.
		snapped := make(map[string]string, len(fieldMapping))
		for _, sourceField := range fieldMapping {
			if val, ok := parsed.Metadata[sourceField]; ok {
				snapped[sourceField] = val
			} else if joined, ok := joinScalarList(parsed.Structured[sourceField]); ok {
				snapped[sourceField] = joined
				delete(parsed.Structured, sourceField)
			}
		}
		// Apply mappings using snapshotted values
		for taiField, sourceField := range fieldMapping {
			if taiField == sourceField {
				continue
			}
			if val, ok := snapped[sourceField]; ok {
				// Explicit tai field value wins over mapping
				if _, exists := parsed.Metadata[taiField]; !exists {
					parsed.Metadata[taiField] = val
				}
				delete(parsed.Metadata, sourceField)
			}
		}
	}

	i.claimNativeFrontmatter(parsed, contentSource)

	if err := validateAssetMetadata(parsed.Metadata, assetType); err != nil {
		return nil, fmt.Errorf("%s %s/%s: %w", assetType, assetName, mainFileName, err)
	}

	resolved.Warnings = append(resolved.Warnings,
		frontmatterWarnings(parsed, assetType, assetName, adap.Name(), adapter.TargetNames())...)

	// Build a synthetic ResolvedAsset so the adapter can transform it
	synthetic := &config.ResolvedAsset{
		Asset: config.Asset{
			Type: assetType,
			Name: mainFileName,
		},
		Content:  parsed.Body,
		Metadata: parsed.Metadata,
		Native:   parsed.Native,
	}

	transformed, err := adap.TransformContent(synthetic)
	if err != nil {
		return nil, fmt.Errorf("transforming %s: %w", mainFileName, err)
	}

	return transformed, nil
}

// joinScalarList joins a YAML list of scalars with ", ", reporting false for
// anything else.
func joinScalarList(v any) (string, bool) {
	items, ok := v.([]any)
	if !ok {
		return "", false
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || frontmatter.IsStructured(item) {
			return "", false
		}
		parts = append(parts, fmt.Sprintf("%v", item))
	}
	return strings.Join(parts, ", "), true
}

// claimNativeFrontmatter hands a file's unknown frontmatter fields to the
// target its source declared with native_frontmatter, if it declared one.
func (i *Installer) claimNativeFrontmatter(parsed *frontmatter.Parsed, sourceName string) {
	if src, ok := i.sources[sourceName]; ok && src.NativeFrontmatter != "" {
		// A retired field is tai's own, not the target's, and must still warn.
		parsed.ClaimUnknownFor(src.NativeFrontmatter, slices.Collect(maps.Keys(retiredFields))...)
	}
}

// findSkillMdFields returns the Fields mapping from the include that targets
// SKILL.md at the root of the skill directory, if any.
func findSkillMdFields(manifest *config.SkillManifest) map[string]string {
	if manifest == nil {
		return nil
	}
	for _, inc := range manifest.Include {
		if isRootSkillMdDest(inc) {
			return inc.Fields
		}
	}
	return nil
}

// isRootSkillMdDest determines if an include's destination resolves to SKILL.md
// at the root of the skill directory.
func isRootSkillMdDest(inc config.Include) bool {
	if strings.HasSuffix(inc.Dest, "/") {
		// Trailing-slash mode: only root-level destinations qualify
		// (i.e., "./" or just "/", not "subdir/")
		destDir := strings.TrimSuffix(inc.Dest, "/")
		if destDir != "" && destDir != "." {
			return false // Subdirectory — not root SKILL.md
		}
		// Check if the source filename is SKILL.md
		if inc.URL != "" {
			parsed, err := url.Parse(inc.URL)
			if err != nil {
				return false
			}
			return path.Base(parsed.Path) == "SKILL.md"
		}
		return filepath.Base(inc.Path) == "SKILL.md"
	}
	// Explicit dest: must be exactly "SKILL.md" (no subdirectory)
	return inc.Dest == "SKILL.md"
}

// restoreBackup restores a backup file.
func (i *Installer) restoreBackup(backupPath, targetPath string) error {
	if err := os.Rename(backupPath, targetPath); err != nil {
		return err
	}
	return nil
}

// cleanEmptyDirs removes empty parent directories up to (but not including) stopAt.
func cleanEmptyDirs(path, stopAt string) {
	dir := filepath.Dir(path)
	for dir != stopAt && dir != "." && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break // Not empty or can't read - stop
		}
		if err := os.Remove(dir); err != nil {
			break // Can't remove - stop
		}
		dir = filepath.Dir(dir)
	}
}

// validateDirTemplates walks a source directory and validates that all files
// with template syntax can be processed successfully. This catches errors in
// Phase 1 before any files are copied to the target directory.
func validateDirTemplates(dirPath string, engine *template.Engine) error {
	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if !template.HasTemplateSyntax(content) {
			return nil
		}

		relPath, _ := filepath.Rel(dirPath, path)

		if template.HasExistingVar(content) {
			return fmt.Errorf("%s: {{ existing }} is not supported in directory assets", relPath)
		}

		if _, err := engine.Process(content, ""); err != nil {
			return fmt.Errorf("processing template in %s: %w", relPath, err)
		}

		return nil
	})
}

// templateProcessDirFiles walks a directory and applies template processing to
// any files containing template syntax. Skips directories and any file whose
// path relative to dirPath skip accepts.
func templateProcessDirFiles(dirPath string, engine *template.Engine, skip func(rel string) bool) error {
	if engine == nil {
		return nil
	}

	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		if skip(relPath) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", relPath, err)
		}

		if !template.HasTemplateSyntax(content) {
			return nil
		}

		if template.HasExistingVar(content) {
			return fmt.Errorf("%s: {{ existing }} is not supported in directory assets", relPath)
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", relPath, err)
		}

		processed, err := engine.Process(content, "")
		if err != nil {
			return fmt.Errorf("processing template in %s: %w", relPath, err)
		}

		return os.WriteFile(path, processed, info.Mode().Perm())
	})
}

// isExcludedPath reports whether rel is one of excludes or lies beneath one.
func isExcludedPath(rel string, excludes []string) bool {
	for _, e := range excludes {
		if e == "." || rel == e || strings.HasPrefix(rel, e+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// copyDirExcluding copies a directory recursively, excluding files with names in excludes.
func copyDirExcluding(src, dst string, excludes ...string) error {
	// Remove existing destination
	_ = os.RemoveAll(dst)

	excludeSet := make(map[string]bool, len(excludes))
	for _, e := range excludes {
		excludeSet[e] = true
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip excluded files (only at top level of skill directory)
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		// Only exclude files at the root of the skill directory
		if excludeSet[relPath] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return pathutil.MkdirPreservingMode(dstPath, d)
		}

		return pathutil.CopyFile(path, dstPath)
	})
}

// Status returns the current installation status for a target.
func (i *Installer) Status(target string) (*Manifest, error) {
	return i.manifestStore.Load(target)
}

// Rebaseline re-hashes the files this install wrote and rewrites the manifest
// so the recorded checksums describe what is actually on disk. It returns the
// target-relative paths whose content changed.
//
// This exists for post_install hooks that post-process managed files - a
// formatter, or a tool injecting machine-local config. Conflict detection
// compares disk against the checksum recorded at write time, so without a
// rebaseline the hook's own edit makes that file conflict on every subsequent
// install, forever. Hook edits are as intentional as the asset content (tai
// invoked the hook, from the user's own profile), so they belong in the
// baseline; conflict detection is there to protect hand edits.
//
// Only files this run wrote are re-hashed. An entry skipped via
// --skip-conflicts deliberately keeps its stale checksum so it keeps
// conflicting until the user resolves it - baselining that would silently
// discard a decision the user made.
//
// Callers must only invoke this after the hook *succeeded*. A hook that
// mutated a file and then failed leaves a partial edit, which is a conflict
// worth surfacing.
func (i *Installer) Rebaseline(adap adapter.Adapter, result *InstallResult) ([]string, error) {
	if result == nil || result.Manifest == nil {
		return nil, nil
	}

	written := make(map[string]bool)
	for _, group := range [][]string{result.Added, result.Restored, result.Updated, result.Unchanged} {
		for _, path := range group {
			written[path] = true
		}
	}

	targetDir := adap.TargetDir()
	var changed []string
	for idx := range result.Manifest.Files {
		entry := &result.Manifest.Files[idx]
		if !written[entry.Path] {
			continue
		}

		fullPath := filepath.Join(targetDir, entry.Path)
		var checksum string
		var err error
		if entry.IsDir {
			checksum, err = ComputeDirChecksum(fullPath)
		} else {
			checksum, err = ComputeFileChecksum(fullPath)
		}
		if err != nil {
			// A file the hook removed is not a conflict - checkConflict treats a
			// missing path as "install it again" - so keep the recorded checksum
			// and carry on rather than failing the rebaseline for the others.
			i.logger.Printf("Could not re-checksum %s after post_install hook (%v); "+
				"keeping the recorded checksum - the next install will reinstall it", entry.Path, err)
			continue
		}

		if checksum != entry.Checksum {
			entry.Checksum = checksum
			changed = append(changed, entry.Path)
		}
	}

	if len(changed) == 0 {
		return nil, nil
	}

	if err := i.manifestStore.Save(result.Manifest); err != nil {
		return changed, fmt.Errorf("saving rebaselined manifest: %w", err)
	}
	return changed, nil
}
