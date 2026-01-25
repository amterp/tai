package install

import (
	"encoding/json"
	"fmt"
	"io"
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
	DryRun        bool              // Preview without making changes
	Force         bool              // Overwrite conflicting files
	SkipConflicts bool              // Leave conflicting files as-is
	Variables     template.Variables // Template variables
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

// Install installs resolved assets to a target.
func (i *Installer) Install(
	adap adapter.Adapter,
	profile string,
	assets []*config.Asset,
	opts InstallOptions,
) (*InstallResult, error) {
	targetDir := adap.TargetDir()
	result := &InstallResult{}

	// Load existing manifest
	existing, err := i.manifestStore.Load(adap.Name())
	if err != nil {
		return nil, fmt.Errorf("loading existing manifest: %w", err)
	}

	// Build map of existing files for quick lookup
	existingFiles := make(map[string]ManifestEntry)
	if existing != nil {
		for _, entry := range existing.Files {
			existingFiles[entry.Path] = entry
		}
	}

	// Build map of existing config entries for quick lookup
	existingConfigEntries := make(map[string]ConfigEntry)
	if existing != nil {
		for _, entry := range existing.ConfigEntries {
			existingConfigEntries[entry.Type] = entry
		}
	}

	// Separate file assets from config assets
	var fileAssets []*config.Asset
	var configAssets []*config.Asset

	supportedConfigTypes := make(map[string]bool)
	for _, t := range adap.SupportedConfigAssetTypes() {
		supportedConfigTypes[t] = true
	}

	for _, asset := range assets {
		if config.IsConfigAssetType(asset.Type) {
			if !supportedConfigTypes[asset.Type] {
				// Unsupported config asset type - warn and skip
				warning := fmt.Sprintf("Asset type '%s' is not supported by target '%s' - skipping %s",
					asset.Type, adap.Name(), asset.Name)
				result.Warnings = append(result.Warnings, warning)
				i.logger.Printf("Warning: %s", warning)
				continue
			}
			configAssets = append(configAssets, asset)
		} else {
			fileAssets = append(fileAssets, asset)
		}
	}

	// Prepare new manifest entries
	var newEntries []ManifestEntry
	var newConfigEntries []ConfigEntry
	templateEngine := template.NewEngine(opts.Variables)

	// Process file assets
	for _, asset := range fileAssets {
		targetPath := adap.MapAssetPath(asset)
		fullTargetPath := filepath.Join(targetDir, targetPath)

		// Resolve the asset to get content
		resolved, err := i.resolveAsset(asset, targetPath, fullTargetPath, templateEngine, adap, opts.DryRun)
		if err != nil {
			return nil, fmt.Errorf("resolving asset %s: %w", asset.SourcePath, err)
		}

		// Check for conflicts
		existingEntry, wasManaged := existingFiles[targetPath]
		conflict := i.checkConflict(fullTargetPath, existingEntry, wasManaged, resolved)

		if conflict != nil {
			if opts.Force {
				// Proceed with overwrite
			} else if opts.SkipConflicts {
				result.Skipped = append(result.Skipped, targetPath)
				// Keep existing entry if it was managed
				if wasManaged {
					newEntries = append(newEntries, existingEntry)
				}
				continue
			} else if !opts.DryRun {
				return nil, fmt.Errorf("conflict at %s: %s (use --force to overwrite or --skip-conflicts to skip)",
					targetPath, conflict.Reason)
			}
		}

		// Install the asset
		if !opts.DryRun {
			if err := i.installAsset(resolved, fullTargetPath, adap); err != nil {
				return nil, fmt.Errorf("installing %s: %w", targetPath, err)
			}

			// For skill directories, re-compute checksum after installation
			// to account for resolved includes
			if resolved.IsDir {
				checksum, err := ComputeDirChecksum(fullTargetPath)
				if err != nil {
					return nil, fmt.Errorf("computing installed checksum for %s: %w", targetPath, err)
				}
				resolved.Checksum = checksum
			}
		}

		// Track what's happening (after checksum is finalized)
		if wasManaged {
			if resolved.Checksum == existingEntry.Checksum {
				result.Unchanged = append(result.Unchanged, targetPath)
			} else {
				result.Updated = append(result.Updated, targetPath)
			}
		} else {
			result.Added = append(result.Added, targetPath)
		}

		newEntries = append(newEntries, EntryFromAsset(resolved))
		delete(existingFiles, targetPath)
	}

	// Process config assets (statusline, etc.)
	// For singleton config types like statusline, later assets override earlier ones
	configByType := make(map[string]*config.Asset)
	for _, asset := range configAssets {
		if prev, exists := configByType[asset.Type]; exists {
			i.logger.Printf("Override: %s/%s overrides %s/%s", asset.Type, asset.Name, prev.Type, prev.Name)
		}
		configByType[asset.Type] = asset
	}

	for assetType, asset := range configByType {
		configEntry, err := i.installConfigAsset(adap, asset, existingConfigEntries, opts)
		if err != nil {
			return nil, fmt.Errorf("installing config asset %s/%s: %w", assetType, asset.Name, err)
		}
		if configEntry != nil {
			existingEntry, wasManaged := existingConfigEntries[assetType]
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
			delete(existingConfigEntries, assetType)
		}
	}

	// Remove files that are no longer in the profile
	for path, entry := range existingFiles {
		fullPath := filepath.Join(targetDir, path)
		result.Removed = append(result.Removed, path)

		if !opts.DryRun {
			// Restore backup if exists
			if entry.BackupPath != "" {
				if err := i.restoreBackup(entry.BackupPath, fullPath); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("restoring backup for %s: %w", path, err))
				}
			} else {
				// Remove the file/directory
				if entry.IsDir {
					if err := os.RemoveAll(fullPath); err != nil && !os.IsNotExist(err) {
						result.Errors = append(result.Errors, fmt.Errorf("removing %s: %w", path, err))
					}
				} else {
					if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
						result.Errors = append(result.Errors, fmt.Errorf("removing %s: %w", path, err))
					}
				}
				// Clean up empty parent directories
				cleanEmptyDirs(fullPath, targetDir)
			}
		}
	}

	// Remove config entries that are no longer in the profile
	for assetType, entry := range existingConfigEntries {
		result.Removed = append(result.Removed, fmt.Sprintf("[%s] %s", assetType, entry.Name))
		if !opts.DryRun {
			if err := i.uninstallConfigEntry(adap, entry); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("removing config %s/%s: %w", assetType, entry.Name, err))
			}
		}
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
	existingConfigEntries map[string]ConfigEntry,
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

	// Check for conflicts with existing unmanaged statusLine
	settingsPath := adap.SettingsFilePath(scope)
	_, wasManaged := existingConfigEntries[config.AssetTypeStatusLine]

	if !wasManaged && !opts.DryRun {
		// Check if settings file has existing statusLine that we don't manage
		existingSettings, err := readJSONFile(settingsPath)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading settings file: %w", err)
		}
		if existingSettings != nil {
			settingsKey := adap.SettingsKey(config.AssetTypeStatusLine)
			if _, hasKey := existingSettings[settingsKey]; hasKey {
				if !opts.Force {
					return nil, fmt.Errorf("found existing %s in %s not managed by tai; use --force to override",
						settingsKey, settingsPath)
				}
			}
		}
	}

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
		// Directory (skill) - compute directory checksum
		checksum, err := ComputeDirChecksum(asset.SourcePath)
		if err != nil {
			return nil, err
		}
		resolved.Checksum = checksum
	} else {
		// File - read content and capture permissions
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
		if template.HasTemplateVars(content) {
			resolved.IsTemplated = true

			// If uses {{existing}}, prepare backup of the existing file
			if template.HasExistingVar(content) {
				if _, err := os.Stat(fullTargetPath); err == nil {
					backupPath := fullTargetPath + ".tai-backup"
					// Only create actual backup if not dry-run
					if !dryRun {
						if err := copyFile(fullTargetPath, backupPath); err != nil {
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

		// Parse and validate frontmatter for all file asset types
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
func (i *Installer) installAsset(resolved *config.ResolvedAsset, fullTargetPath string, adap adapter.Adapter) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullTargetPath), 0755); err != nil {
		return err
	}

	if resolved.IsDir {
		// For skill directories, exclude the manifest file (it's a build-time artifact)
		if err := copyDirExcluding(resolved.SourcePath, fullTargetPath, SkillManifestFilename); err != nil {
			return err
		}

		// Load and process skill manifest if present
		manifest, err := config.LoadSkillManifest(resolved.SourcePath)
		if err != nil {
			return fmt.Errorf("loading skill manifest: %w", err)
		}
		if manifest != nil && len(manifest.Include) > 0 {
			resolver := NewIncludeResolver(i.sources, i.httpClient)
			if err := resolver.ResolveIncludes(manifest, fullTargetPath); err != nil {
				return fmt.Errorf("resolving includes: %w", err)
			}
		}

		// Detect field mapping for SKILL.md from includes
		fieldMapping := findSkillMdFields(manifest)

		// Transform SKILL.md with target-specific frontmatter
		if err := i.transformSkillMd(resolved, fullTargetPath, adap, fieldMapping); err != nil {
			return err
		}

		return nil
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

// transformSkillMd reads SKILL.md from the installed skill directory, parses
// and validates its tai frontmatter, transforms it through the adapter, and
// writes the result back. This gives skill SKILL.md files target-specific
// frontmatter (name, description, tools) just like standalone file assets.
//
// If fieldMapping is non-nil, it remaps source field names to tai field names
// before validation — e.g., {"name": "title"} renames the source's "title"
// field to tai's "name" field.
func (i *Installer) transformSkillMd(resolved *config.ResolvedAsset, skillTargetPath string, adap adapter.Adapter, fieldMapping map[string]string) error {
	skillMdPath := filepath.Join(skillTargetPath, "SKILL.md")
	info, err := os.Stat(skillMdPath)
	if os.IsNotExist(err) {
		return nil // No SKILL.md — nothing to transform
	}
	if err != nil {
		return fmt.Errorf("reading SKILL.md: %w", err)
	}
	mode := info.Mode().Perm()

	content, err := os.ReadFile(skillMdPath)
	if err != nil {
		return fmt.Errorf("reading SKILL.md: %w", err)
	}

	parsed, err := frontmatter.Parse(content)
	if err != nil {
		return fmt.Errorf("parsing SKILL.md frontmatter: %w", err)
	}

	// Apply field mapping: renames source field names to tai field names.
	// We snapshot values first to avoid non-determinism from map iteration
	// order when mappings reference each other's fields.
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

	if err := validateAssetMetadata(parsed.Metadata, config.AssetTypeSkill); err != nil {
		return fmt.Errorf("skill %s/SKILL.md: %w", resolved.Name, err)
	}

	// Build a synthetic ResolvedAsset so the adapter can transform it
	synthetic := &config.ResolvedAsset{
		Asset: config.Asset{
			Type: config.AssetTypeSkill,
			Name: "SKILL.md",
		},
		Content:  parsed.Body,
		Metadata: parsed.Metadata,
	}

	transformed, err := adap.TransformContent(synthetic)
	if err != nil {
		return fmt.Errorf("transforming SKILL.md: %w", err)
	}

	return os.WriteFile(skillMdPath, transformed, mode)
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

// mkdirPreservingMode creates a directory at dstPath using the permissions from the source DirEntry.
func mkdirPreservingMode(dstPath string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	return os.MkdirAll(dstPath, info.Mode().Perm())
}

// copyFile copies a file from src to dst, preserving the source file's permissions.
func copyFile(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Close()
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
			return mkdirPreservingMode(dstPath, d)
		}

		return copyFile(path, dstPath)
	})
}

// Status returns the current installation status for a target.
func (i *Installer) Status(target string) (*Manifest, error) {
	return i.manifestStore.Load(target)
}
