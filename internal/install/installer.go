package install

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/template"
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
	Added    []string // Files added
	Updated  []string // Files updated
	Removed  []string // Files removed
	Skipped  []string // Files skipped due to conflicts
	Errors   []error  // Non-fatal errors
	Manifest *Manifest
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
}

// NewInstaller creates a new installer.
func NewInstaller(manifestStore *ManifestStore, configRepo string) *Installer {
	return &Installer{
		manifestStore: manifestStore,
		configRepo:    configRepo,
	}
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

	// Prepare new manifest entries
	var newEntries []ManifestEntry
	templateEngine := template.NewEngine(opts.Variables)

	// Process each asset
	for _, asset := range assets {
		targetPath := adap.MapAssetPath(asset)
		fullTargetPath := filepath.Join(targetDir, targetPath)

		// Resolve the asset to get content
		resolved, err := i.resolveAsset(asset, targetPath, fullTargetPath, templateEngine)
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

		// Track what's happening
		if wasManaged {
			result.Updated = append(result.Updated, targetPath)
		} else {
			result.Added = append(result.Added, targetPath)
		}

		// Install the asset
		if !opts.DryRun {
			if err := i.installAsset(resolved, fullTargetPath); err != nil {
				return nil, fmt.Errorf("installing %s: %w", targetPath, err)
			}
		}

		newEntries = append(newEntries, EntryFromAsset(resolved))
		delete(existingFiles, targetPath)
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
			}
		}
	}

	// Build and save new manifest
	result.Manifest = BuildManifest(adap.Name(), profile, i.configRepo, newEntries)
	if !opts.DryRun {
		if err := i.manifestStore.Save(result.Manifest); err != nil {
			return nil, fmt.Errorf("saving manifest: %w", err)
		}
	}

	return result, nil
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
		}
	}

	// Delete manifest
	if err := i.manifestStore.Delete(adap.Name()); err != nil {
		result.Errors = append(result.Errors, err)
	}

	return result, nil
}

// resolveAsset resolves an asset to a ResolvedAsset with content loaded.
func (i *Installer) resolveAsset(
	asset *config.Asset,
	targetPath, fullTargetPath string,
	engine *template.Engine,
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
		// File - read content
		content, err := os.ReadFile(asset.SourcePath)
		if err != nil {
			return nil, err
		}

		// Check if templating is needed
		if template.HasTemplateVars(content) {
			resolved.IsTemplated = true

			// If uses {{existing}}, backup the existing file
			if template.HasExistingVar(content) {
				if _, err := os.Stat(fullTargetPath); err == nil {
					backupPath := fullTargetPath + ".tai-backup"
					if err := copyFile(fullTargetPath, backupPath); err != nil {
						return nil, fmt.Errorf("creating backup: %w", err)
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

		resolved.Content = content
		resolved.Checksum = ComputeChecksum(content)
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
func (i *Installer) installAsset(resolved *config.ResolvedAsset, fullTargetPath string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullTargetPath), 0755); err != nil {
		return err
	}

	if resolved.IsDir {
		// Copy directory recursively
		return copyDir(resolved.SourcePath, fullTargetPath)
	}

	// Write file content
	return os.WriteFile(fullTargetPath, resolved.Content, 0644)
}

// restoreBackup restores a backup file.
func (i *Installer) restoreBackup(backupPath, targetPath string) error {
	if err := os.Rename(backupPath, targetPath); err != nil {
		return err
	}
	return nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// copyDir copies a directory recursively.
func copyDir(src, dst string) error {
	// Remove existing destination
	os.RemoveAll(dst)

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		return copyFile(path, dstPath)
	})
}

// Status returns the current installation status for a target.
func (i *Installer) Status(target string) (*Manifest, error) {
	return i.manifestStore.Load(target)
}
