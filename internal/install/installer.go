package install

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
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
}

// InstallResult contains the result of an installation.
type InstallResult struct {
	Added     []string // Files added
	Updated   []string // Files updated (content changed)
	Unchanged []string // Files unchanged (content identical)
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
		httpClient:    DefaultHTTPClient(),
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
	conflict      *Conflict
}

// installContext holds shared state during the two-phase install process.
// This avoids threading many parameters through helper functions.
type installContext struct {
	adap                  adapter.Adapter
	targetDir             string
	templateEngine        *template.Engine
	existingFiles         map[string]ManifestEntry
	existingConfigEntries map[string]ConfigEntry
	opts                  InstallOptions
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
		return nil, err
	}

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
			existingConfigEntries[entry.Type] = entry
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

	return &installContext{
		adap:                  adap,
		targetDir:             adap.TargetDir(),
		templateEngine:        engine,
		existingFiles:         existingFiles,
		existingConfigEntries: existingConfigEntries,
		opts:                  opts,
	}
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
) ([]resolvedFileAsset, map[string]*config.Asset, []ConflictDetail, map[string]bool, error) {
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

	// Resolve and check config assets
	// For singleton config types like statusline, later assets override earlier ones
	configByType := make(map[string]*config.Asset)
	configConflicts := make(map[string]bool)

	for _, asset := range configAssets {
		if prev, exists := configByType[asset.Type]; exists {
			i.logger.Printf("Override: %s/%s overrides %s/%s", asset.Type, asset.Name, prev.Type, prev.Name)
		}
		configByType[asset.Type] = asset
	}

	for assetType := range configByType {
		conflict := i.checkConfigConflict(ctx.adap, assetType, ctx.existingConfigEntries)
		if conflict != nil {
			conflicts = append(conflicts, *conflict)
			configConflicts[assetType] = true
		}
	}

	return resolvedAssets, configByType, conflicts, configConflicts, nil
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
	resolved, err := i.resolveAsset(workingAsset, targetPath, fullTargetPath, ctx.templateEngine, ctx.adap, true)
	if err != nil {
		return resolvedFileAsset{}, nil, fmt.Errorf("resolving asset %s: %w", workingAsset.SourcePath, err)
	}

	existingEntry, wasManaged := ctx.existingFiles[targetPath]
	conflict := i.checkConflict(fullTargetPath, existingEntry, wasManaged, resolved)

	ra := resolvedFileAsset{
		asset:         asset,
		workingAsset:  workingAsset,
		targetPath:    targetPath,
		fullPath:      fullTargetPath,
		resolved:      resolved,
		existingEntry: existingEntry,
		wasManaged:    wasManaged,
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
	configByType map[string]*config.Asset,
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

	// Install config assets
	for assetType, asset := range configByType {
		if configConflicts[assetType] {
			if opts.Force {
				// Proceed with overwrite
			} else if opts.SkipConflicts {
				result.Skipped = append(result.Skipped, fmt.Sprintf("[%s] %s", assetType, asset.Name))
				continue
			}
		}

		configEntry, err := i.installConfigAsset(ctx.adap, asset, ctx.existingConfigEntries, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("installing config asset %s/%s: %w", assetType, asset.Name, err)
		}
		if configEntry != nil {
			existingEntry, wasManaged := ctx.existingConfigEntries[assetType]
			if wasManaged {
				if configEntry.Checksum == existingEntry.Checksum {
					result.Unchanged = append(result.Unchanged, fmt.Sprintf("[%s] %s", assetType, asset.Name))
				} else {
					result.Updated = append(result.Updated, fmt.Sprintf("[%s] %s", assetType, asset.Name))
				}
			} else {
				result.Added = append(result.Added, fmt.Sprintf("[%s] %s", assetType, asset.Name))
			}
			newConfigEntries = append(newConfigEntries, *configEntry)
			delete(ctx.existingConfigEntries, assetType)
		}
	}

	// Remove files that are no longer in the profile
	if err := i.removeStaleFiles(ctx, result); err != nil {
		return nil, nil, err
	}

	// Remove config entries that are no longer in the profile
	i.removeStaleConfigEntries(ctx, result)

	return newEntries, newConfigEntries, nil
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

	// Install the asset
	if !opts.DryRun {
		if err := i.installAsset(ra.resolved, ra.fullPath, ctx.adap, ctx.templateEngine); err != nil {
			return ManifestEntry{}, false, fmt.Errorf("installing %s: %w", ra.targetPath, err)
		}

		// For skill directories, re-compute checksum after installation
		// to account for resolved includes
		if ra.resolved.IsDir {
			checksum, err := ComputeDirChecksum(ra.fullPath)
			if err != nil {
				return ManifestEntry{}, false, fmt.Errorf("computing installed checksum for %s: %w", ra.targetPath, err)
			}
			ra.resolved.Checksum = checksum
		}
	}

	// Track what's happening
	if ra.wasManaged {
		if ra.resolved.Checksum == ra.existingEntry.Checksum {
			result.Unchanged = append(result.Unchanged, ra.targetPath)
		} else {
			result.Updated = append(result.Updated, ra.targetPath)
		}
	} else {
		result.Added = append(result.Added, ra.targetPath)
	}

	return EntryFromAsset(ra.resolved), false, nil
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
	for assetType, entry := range ctx.existingConfigEntries {
		result.Removed = append(result.Removed, fmt.Sprintf("[%s] %s", assetType, entry.Name))
		if !ctx.opts.DryRun {
			if err := i.uninstallConfigEntry(ctx.adap, entry); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("removing config %s/%s: %w", assetType, entry.Name, err))
			}
		}
	}
}

// checkConfigConflict checks if a config asset type has an unmanaged entry in the settings file.
// Returns a ConflictDetail if there's a conflict, nil otherwise.
func (i *Installer) checkConfigConflict(
	adap adapter.Adapter,
	assetType string,
	existingConfigEntries map[string]ConfigEntry,
) *ConflictDetail {
	scope := "user" // TODO: get from profile options

	_, wasManaged := existingConfigEntries[assetType]
	if wasManaged {
		return nil // Already managed by tai, not a conflict
	}

	settingsPath := adap.SettingsFilePath(scope)
	existingSettings, err := readJSONFile(settingsPath)
	if err != nil || existingSettings == nil {
		return nil // No settings file or can't read it
	}

	settingsKey := adap.SettingsKey(assetType)
	if settingsKey == "" {
		return nil // Unknown asset type
	}

	if _, hasKey := existingSettings[settingsKey]; hasKey {
		return &ConflictDetail{
			Path:   fmt.Sprintf("[%s]", assetType),
			Reason: fmt.Sprintf("unmanaged %s exists in %s", settingsKey, settingsPath),
			IsDir:  false,
		}
	}

	return nil
}

// installConfigAsset installs a config asset (statusline, etc.) to the target's settings file.
//
// Tech debt note: This switch-statement approach requires modification in multiple places
// when adding new config asset types (MCPs, hooks in v0.2). If this pattern proves unwieldy
// with 2-3 config types, consider refactoring to a handler registry pattern where each
// config type registers its own install/uninstall handlers.
func (i *Installer) installConfigAsset(
	adap adapter.Adapter,
	asset *config.Asset,
	existingConfigEntries map[string]ConfigEntry,
	opts InstallOptions,
) (*ConfigEntry, error) {
	scope := "user" // TODO: get from profile options

	switch asset.Type {
	case config.AssetTypeStatusLine:
		return i.installStatusLine(adap, asset, existingConfigEntries, scope, opts)
	default:
		return nil, fmt.Errorf("unsupported config asset type: %s", asset.Type)
	}
}

// installStatusLine installs a status line config to the target's settings file.
func (i *Installer) installStatusLine(
	adap adapter.Adapter,
	asset *config.Asset,
	_ map[string]ConfigEntry, // existingConfigEntries - unused; conflict check moved to phase 1
	scope string,
	opts InstallOptions,
) (*ConfigEntry, error) {
	// Parse the status line config
	cfg, rawNative, err := config.ParseStatusLineConfig(asset.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("parsing status line config: %w", err)
	}

	// Validate if it's tai format
	if cfg != nil {
		if err := validateStatusLineConfig(cfg); err != nil {
			return nil, fmt.Errorf("validating status line config: %w", err)
		}
	} else {
		i.logger.Printf("Using native format status line (not tai format): %s", asset.Name)
	}

	// Transform to target format
	statusLineData, err := adap.TransformStatusLine(cfg, rawNative)
	if err != nil {
		return nil, fmt.Errorf("transforming status line: %w", err)
	}

	// Compute checksum of the transformed config
	configJSON, err := json.Marshal(statusLineData)
	if err != nil {
		return nil, fmt.Errorf("serializing status line config: %w", err)
	}
	checksum := ComputeChecksum(configJSON)

	// Note: Conflict checking for config assets is now done upfront in Install() phase 1.
	// If we get here, either there was no conflict, or Force/SkipConflicts was set.

	settingsPath := adap.SettingsFilePath(scope)

	// Install to settings file
	if !opts.DryRun {
		settingsKey := adap.SettingsKey(config.AssetTypeStatusLine)
		if err := i.writeConfigToSettings(settingsPath, settingsKey, statusLineData); err != nil {
			return nil, fmt.Errorf("writing status line to settings: %w", err)
		}
	}

	return &ConfigEntry{
		Type:         config.AssetTypeStatusLine,
		Name:         asset.Name,
		SourceName:   asset.SourceName,
		SourcePath:   asset.SourcePath,
		SettingsFile: settingsPath,
		Checksum:     checksum,
	}, nil
}

// uninstallConfigEntry removes a config entry from the target's settings file.
func (i *Installer) uninstallConfigEntry(adap adapter.Adapter, entry ConfigEntry) error {
	settingsKey := adap.SettingsKey(entry.Type)
	if settingsKey == "" {
		return fmt.Errorf("unsupported config asset type: %s", entry.Type)
	}
	return i.removeConfigFromSettings(entry.SettingsFile, settingsKey)
}

// writeConfigToSettings writes a config value to a settings JSON file.
func (i *Installer) writeConfigToSettings(path, key string, value any) error {
	settings, err := readJSONFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	settings[key] = value

	return writeJSONFile(path, settings)
}

// removeConfigFromSettings removes a config key from a settings JSON file.
func (i *Installer) removeConfigFromSettings(path, key string) error {
	settings, err := readJSONFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to remove
		}
		return err
	}

	delete(settings, key)

	// If settings is empty, we could remove the file, but it's safer to leave it
	return writeJSONFile(path, settings)
}

// readJSONFile reads and parses a JSON file.
func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	return result, nil
}

// writeJSONFile writes a map to a JSON file with nice formatting.
func writeJSONFile(path string, data map[string]any) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("serializing JSON: %w", err)
	}

	// Add trailing newline
	content = append(content, '\n')

	return os.WriteFile(path, content, 0644)
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

	// Remove config entries
	for _, entry := range manifest.ConfigEntries {
		result.Removed = append(result.Removed, fmt.Sprintf("[%s] %s", entry.Type, entry.Name))
		if err := i.uninstallConfigEntry(adap, entry); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("removing config %s/%s: %w", entry.Type, entry.Name, err))
		}
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

		// Check if templating is needed
		if template.HasTemplateSyntax(content) {
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

		if err := validateAssetMetadata(parsed.Metadata, asset.Type); err != nil {
			return nil, fmt.Errorf("asset %s/%s: %w", asset.Type, asset.Name, err)
		}

		resolved.Metadata = parsed.Metadata
		resolved.Content = parsed.Body

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

// checkConflict checks for conflicts when installing a file.
func (i *Installer) checkConflict(
	fullPath string,
	existingEntry ManifestEntry,
	wasManaged bool,
	resolved *config.ResolvedAsset,
) *Conflict {
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return nil // No conflict - file doesn't exist
	}
	if err != nil {
		return &Conflict{Path: fullPath, Reason: fmt.Sprintf("stat error: %v", err)}
	}

	if !wasManaged {
		// File exists but wasn't managed by tai
		// However, if the asset uses {{existing}}, it's designed to wrap existing content
		if resolved.BackupPath != "" {
			return nil // Not a conflict - we're wrapping existing content
		}
		return &Conflict{Path: fullPath, Reason: "unmanaged file exists"}
	}

	// Check if user modified the file
	var currentChecksum string
	if info.IsDir() {
		currentChecksum, _ = ComputeDirChecksum(fullPath)
	} else {
		currentChecksum, _ = ComputeFileChecksum(fullPath)
	}

	if currentChecksum != existingEntry.Checksum {
		return &Conflict{Path: fullPath, Reason: "file modified since last install"}
	}

	return nil
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
	if err := templateProcessDirFiles(fullTargetPath, engine, mainFileName); err != nil {
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
	if manifest != nil && len(manifest.Include) > 0 {
		resolver := NewIncludeResolver(i.sources, i.httpClient)
		resolver.SetCommandContext(CommandContext{
			AssetType:   config.AssetTypeSkill,
			AssetName:   resolved.Name,
			TargetName:  adap.Name(),
			ManifestDir: resolved.SourcePath,
			Version:     i.version,
		})
		if err := resolver.ResolveIncludes(manifest, fullTargetPath); err != nil {
			return fmt.Errorf("resolving includes: %w", err)
		}
	}

	// Template-process non-main files in the copied directory
	if err := templateProcessDirFiles(fullTargetPath, engine, mainFileName); err != nil {
		return err
	}

	// Detect field mapping for SKILL.md from includes
	fieldMapping := findSkillMdFields(manifest)

	// Transform SKILL.md from installed location (after includes may have replaced it)
	if err := i.transformMainFileInPlace(resolved, fullTargetPath, adap, mainFileName, fieldMapping, engine); err != nil {
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

	transformed, err := i.transformMainFileContent(content, resolved.Type, resolved.Name, mainFileName, nil, adap, engine)
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
func (i *Installer) transformMainFileInPlace(resolved *config.ResolvedAsset, dirTargetPath string, adap adapter.Adapter, mainFileName string, fieldMapping map[string]string, engine *template.Engine) error {
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

	transformed, err := i.transformMainFileContent(content, resolved.Type, resolved.Name, mainFileName, fieldMapping, adap, engine)
	if err != nil {
		return err
	}

	return os.WriteFile(mainFilePath, transformed, mode)
}

// transformMainFileContent parses, validates, and transforms main file content.
// This is the shared logic used by both transformMainFileFromSource and transformMainFileInPlace.
func (i *Installer) transformMainFileContent(content []byte, assetType, assetName, mainFileName string, fieldMapping map[string]string, adap adapter.Adapter, engine *template.Engine) ([]byte, error) {
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
		// Snapshot source values before mutating
		snapped := make(map[string]string, len(fieldMapping))
		for _, sourceField := range fieldMapping {
			if val, ok := parsed.Metadata[sourceField]; ok {
				snapped[sourceField] = val
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

	if err := validateAssetMetadata(parsed.Metadata, assetType); err != nil {
		return nil, fmt.Errorf("%s %s/%s: %w", assetType, assetName, mainFileName, err)
	}

	// Build a synthetic ResolvedAsset so the adapter can transform it
	synthetic := &config.ResolvedAsset{
		Asset: config.Asset{
			Type: assetType,
			Name: mainFileName,
		},
		Content:  parsed.Body,
		Metadata: parsed.Metadata,
	}

	transformed, err := adap.TransformContent(synthetic)
	if err != nil {
		return nil, fmt.Errorf("transforming %s: %w", mainFileName, err)
	}

	return transformed, nil
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
// any files containing template syntax. Skips directories and excluded files.
// Excluded files are matched by their path relative to dirPath (e.g. "SKILL.md"
// excludes only a root-level file, not "subdir/SKILL.md").
func templateProcessDirFiles(dirPath string, engine *template.Engine, excludeFiles ...string) error {
	if engine == nil {
		return nil
	}

	excludeSet := make(map[string]bool, len(excludeFiles))
	for _, e := range excludeFiles {
		excludeSet[e] = true
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
		if excludeSet[relPath] {
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
	for _, group := range [][]string{result.Added, result.Updated, result.Unchanged} {
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
