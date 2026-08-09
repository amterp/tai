// Package install handles installation and manifest management.
package install

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
)

// Manifest tracks what tai has installed to a target.
type Manifest struct {
	Version       int               `yaml:"version"`        // Schema version
	Target        string            `yaml:"target"`
	Profile       string            `yaml:"profile"`
	ConfigRepo    string            `yaml:"config_repo"`
	InstalledAt   time.Time         `yaml:"installed_at"`
	Files         []ManifestEntry   `yaml:"files"`
	ConfigEntries []ConfigEntry     `yaml:"config_entries,omitempty"` // Config assets (statusline, etc.)
}

// ManifestEntry represents a single installed file.
type ManifestEntry struct {
	Path       string `yaml:"path"`        // Target-relative path
	SourceName string `yaml:"source"`      // Source name
	SourcePath string `yaml:"source_path"` // Full filesystem path in source
	Checksum   string `yaml:"checksum"`    // SHA-256 checksum
	BackupPath string `yaml:"backup_path"` // Path to backup (for {{existing}} files)
	IsDir      bool   `yaml:"is_dir"`      // True for skill directories
}

// ConfigEntry represents an installed config asset (statusline, mcp, hook, etc.).
// Config assets are installed as entries in target config files rather than
// as standalone files.
type ConfigEntry struct {
	Type         string `yaml:"type"`          // Asset type (e.g., "statusline")
	Name         string `yaml:"name"`          // Asset name (filename, extension stripped)
	SourceName   string `yaml:"source"`        // Source name
	SourcePath   string `yaml:"source_path"`   // Full filesystem path in source
	SettingsFile string `yaml:"settings_file"` // Primary config file this was installed to
	Scope        string `yaml:"scope,omitempty"`
	Checksum     string `yaml:"checksum"` // SHA-256 checksum of the installed config

	// Edits records exactly what tai wrote, so uninstall can take back those
	// entries and nothing else. Config files are shared - a settings.json holds
	// hooks from other tools beside tai's, and ~/.claude.json holds a session's
	// whole state - so removal has to be surgical.
	//
	// Empty for entries written before tai tracked edits; those fall back to
	// removing the whole settings key, which is what installed them.
	Edits []configasset.AppliedEdit `yaml:"edits,omitempty"`
}

// Key returns the manifest identity of a config entry. Collection types hold
// many entries per type and so are keyed by name; singletons are keyed by type
// alone, which keeps a renamed status line asset from installing twice.
func (e ConfigEntry) Key() string {
	return ConfigEntryKey(e.Type, e.Name)
}

// withoutEdits returns a copy holding only the edits not present in keep, so a
// reinstall removes what it is not about to rewrite in place.
func (e ConfigEntry) withoutEdits(keep map[editSlot]configasset.AppliedEdit) ConfigEntry {
	remaining := make([]configasset.AppliedEdit, 0, len(e.Edits))
	for _, a := range e.Edits {
		if _, kept := keep[slotOf(a.File, a.KeyPath, a.Mode, a.ID)]; !kept {
			remaining = append(remaining, a)
		}
	}
	e.Edits = remaining
	return e
}

// ConfigEntryKey builds the lookup key for a config asset of the given type.
func ConfigEntryKey(assetType, name string) string {
	if configasset.IsCollection(assetType) {
		return assetType + "/" + name
	}
	return assetType
}

// ManifestStore handles loading and saving manifests.
type ManifestStore struct {
	manifestDir string // e.g., ~/.config/tai/manifests
}

// NewManifestStore creates a new manifest store.
func NewManifestStore(manifestDir string) *ManifestStore {
	return &ManifestStore{manifestDir: manifestDir}
}

// Load loads a manifest for a target.
func (s *ManifestStore) Load(target string) (*Manifest, error) {
	path := s.manifestPath(target)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil // No existing manifest
	}
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return &manifest, nil
}

// Save saves a manifest for a target atomically.
// Writes to a temp file first, then renames to ensure atomic update.
func (s *ManifestStore) Save(manifest *Manifest) error {
	if err := os.MkdirAll(s.manifestDir, 0755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("serializing manifest: %w", err)
	}

	finalPath := s.manifestPath(manifest.Target)
	tempPath := finalPath + ".tmp"

	// Write to temp file
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath) // Clean up temp file on failure
		return fmt.Errorf("finalizing manifest: %w", err)
	}

	return nil
}

// Delete deletes a manifest for a target.
func (s *ManifestStore) Delete(target string) error {
	path := s.manifestPath(target)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting manifest: %w", err)
	}
	return nil
}

// List returns the names of all targets that have manifests installed.
func (s *ManifestStore) List() ([]string, error) {
	entries, err := os.ReadDir(s.manifestDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading manifests directory: %w", err)
	}

	var targets []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") {
			targets = append(targets, strings.TrimSuffix(name, ".yaml"))
		}
	}
	sort.Strings(targets)
	return targets, nil
}

// manifestPath returns the path to a manifest file.
func (s *ManifestStore) manifestPath(target string) string {
	return filepath.Join(s.manifestDir, target+".yaml")
}

// ComputeChecksum computes the SHA-256 checksum of file content.
func ComputeChecksum(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ComputeFileChecksum computes the checksum of a file.
func ComputeFileChecksum(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return ComputeChecksum(content), nil
}

// ComputeDirChecksum computes a checksum for a directory (recursive file listing).
// Files are sorted by path to ensure deterministic checksums across platforms.
func ComputeDirChecksum(path string) (string, error) {
	// First, collect all file paths
	var files []string
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(path, p)
			files = append(files, relPath)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	// Sort paths for deterministic ordering
	sort.Strings(files)

	// Now hash in sorted order
	hash := sha256.New()
	for _, relPath := range files {
		fullPath := filepath.Join(path, relPath)

		// Include relative path in hash
		hash.Write([]byte(relPath))

		// Include file content
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return "", err
		}
		hash.Write(content)
	}

	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// ManifestVersion is the current manifest schema version.
const ManifestVersion = 1

// BuildManifest builds a manifest from resolved assets.
func BuildManifest(target, profile, configRepo string, entries []ManifestEntry, configEntries []ConfigEntry) *Manifest {
	return &Manifest{
		Version:       ManifestVersion,
		Target:        target,
		Profile:       profile,
		ConfigRepo:    configRepo,
		InstalledAt:   time.Now(),
		Files:         entries,
		ConfigEntries: configEntries,
	}
}

// EntryFromAsset creates a manifest entry from a resolved asset.
func EntryFromAsset(asset *config.ResolvedAsset) ManifestEntry {
	return ManifestEntry{
		Path:       asset.TargetPath,
		SourceName: asset.SourceName,
		SourcePath: asset.SourcePath,
		Checksum:   asset.Checksum,
		BackupPath: asset.BackupPath,
		IsDir:      asset.IsDir,
	}
}
