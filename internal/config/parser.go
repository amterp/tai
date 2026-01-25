package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseTaiConfig parses a tai.yaml file from the given path.
func ParseTaiConfig(path string) (*TaiConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading tai.yaml: %w", err)
	}

	var config TaiConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing tai.yaml: %w", err)
	}

	if err := validateTaiConfig(&config); err != nil {
		return nil, err
	}

	// Ensure implicit local source exists
	hasLocal := false
	for _, s := range config.Sources {
		if s.Name == "local" {
			hasLocal = true
			break
		}
	}
	if !hasLocal {
		// Add implicit local source at the beginning (lowest priority)
		implicitLocal := Source{
			Name: "local",
			Type: "local",
			Path: "./assets",
		}
		config.Sources = append([]Source{implicitLocal}, config.Sources...)
	}

	return &config, nil
}

// validateTaiConfig validates the parsed configuration.
func validateTaiConfig(config *TaiConfig) error {
	if config.Version != 1 {
		return fmt.Errorf("unsupported config version: %d (expected 1)", config.Version)
	}

	seen := make(map[string]bool)
	for i, s := range config.Sources {
		if s.Name == "" {
			return fmt.Errorf("source[%d]: missing name", i)
		}
		if seen[s.Name] {
			return fmt.Errorf("duplicate source name: %s", s.Name)
		}
		seen[s.Name] = true

		if s.Path == "" {
			return fmt.Errorf("source %q: missing path", s.Name)
		}

		// Normalize type
		if s.Type == "" {
			config.Sources[i].Type = "local"
		} else if s.Type != "local" && s.Type != "tai-repo" {
			return fmt.Errorf("source %q: invalid type %q (expected 'local' or 'tai-repo')", s.Name, s.Type)
		}
	}

	return nil
}

// ParseBundle parses a bundle.yaml file from the given path.
func ParseBundle(path string) (*Bundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading bundle file: %w", err)
	}

	var bundle Bundle
	if err := yaml.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("parsing bundle file: %w", err)
	}

	if err := validateBundle(&bundle, path); err != nil {
		return nil, err
	}

	return &bundle, nil
}

// validateBundle validates a parsed bundle.
func validateBundle(bundle *Bundle, path string) error {
	if bundle.Name == "" {
		return fmt.Errorf("bundle at %s: missing name", path)
	}

	for i, entry := range bundle.Assets {
		if err := validateAssetEntry(entry, i); err != nil {
			return fmt.Errorf("bundle %q: %w", bundle.Name, err)
		}
	}

	return nil
}

// ParseProfile parses a profile.yaml file from the given path.
func ParseProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile file: %w", err)
	}

	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parsing profile file: %w", err)
	}

	if err := validateProfile(&profile, path); err != nil {
		return nil, err
	}

	return &profile, nil
}

// validateProfile validates a parsed profile.
func validateProfile(profile *Profile, path string) error {
	if profile.Name == "" {
		return fmt.Errorf("profile at %s: missing name", path)
	}
	if profile.Target == "" {
		return fmt.Errorf("profile %q: missing target", profile.Name)
	}
	if len(profile.Bundles) == 0 && len(profile.Assets) == 0 {
		return fmt.Errorf("profile %q: must specify at least one bundle or asset", profile.Name)
	}

	for i, entry := range profile.Assets {
		if err := validateAssetEntry(entry, i); err != nil {
			return fmt.Errorf("profile %q: %w", profile.Name, err)
		}
	}

	return nil
}

// validateAssetEntry validates a single asset entry.
func validateAssetEntry(entry AssetEntry, index int) error {
	if entry.Type == "" {
		return fmt.Errorf("asset[%d]: missing type", index)
	}

	// Provide helpful migration message for removed "doc" type
	if entry.Type == "doc" {
		return fmt.Errorf("asset[%d]: type 'doc' has been removed; use 'file' with 'dest: \"\"' for root-level files", index)
	}

	if !IsValidAssetType(entry.Type) {
		return fmt.Errorf("asset[%d]: invalid type %q", index, entry.Type)
	}
	if entry.Path == "" {
		return fmt.Errorf("asset[%d]: missing path", index)
	}

	// Validate dest field
	if entry.Type == AssetTypeFile {
		// For file type, dest specifies where the file lands in the target.
		// Empty string means target root (e.g., ~/.claude/), which is valid.
		// In YAML, both missing and explicit empty string result in "", so we
		// can't distinguish them. This is acceptable: users who forget dest
		// get root-level installation, which is documented as the default.

		// Validate dest path if non-empty
		if entry.Dest != "" {
			if err := validateDestPath(entry.Dest); err != nil {
				return fmt.Errorf("asset[%d]: %w", index, err)
			}
		}
	} else if entry.Dest != "" {
		// dest is only valid for file type
		return fmt.Errorf("asset[%d]: 'dest' field is only valid for 'file' type, not %q", index, entry.Type)
	}

	return nil
}

// validateDestPath validates a destination path.
func validateDestPath(dest string) error {
	// Reject absolute paths
	if strings.HasPrefix(dest, "/") {
		return fmt.Errorf("dest cannot be an absolute path: %q", dest)
	}

	// Reject path traversal (.. as a path segment, not as part of a filename)
	for _, segment := range strings.Split(dest, "/") {
		if segment == ".." {
			return fmt.Errorf("dest cannot contain path traversal (..): %q", dest)
		}
	}

	return nil
}

// ParseAssetReference parses an asset reference string into source and path.
// Format: "source:path" or just "path" (defaults to "local" source).
func ParseAssetReference(ref string) (sourceName, assetPath string) {
	if idx := strings.Index(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:]
	}
	return "local", ref
}

// LoadBundlesFromDir loads all bundle files from a directory.
func LoadBundlesFromDir(dir string) (map[string]*Bundle, error) {
	bundles := make(map[string]*Bundle)

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return bundles, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading bundles directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		path := filepath.Join(dir, name)
		bundle, err := ParseBundle(path)
		if err != nil {
			return nil, err
		}

		if existing, ok := bundles[bundle.Name]; ok {
			return nil, fmt.Errorf("duplicate bundle name %q: defined in both %s and %s",
				bundle.Name, path, existing.Name)
		}
		bundles[bundle.Name] = bundle
	}

	return bundles, nil
}

// ParseStatusLineConfig parses a status line config YAML file.
// Returns the parsed config and a flag indicating if it's in tai format.
// If not tai format, the raw YAML content should be passed through to the target.
func ParseStatusLineConfig(path string) (*StatusLineConfig, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading status line config: %w", err)
	}

	var cfg StatusLineConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parsing status line config: %w", err)
	}

	// If no level field, treat as native format (pass through raw data)
	if !cfg.IsTaiFormat() {
		return nil, data, nil
	}

	return &cfg, nil, nil
}

// LoadProfilesFromDir loads all profile files from a directory.
func LoadProfilesFromDir(dir string) (map[string]*Profile, error) {
	profiles := make(map[string]*Profile)

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return profiles, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading profiles directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		path := filepath.Join(dir, name)
		profile, err := ParseProfile(path)
		if err != nil {
			return nil, err
		}

		if existing, ok := profiles[profile.Name]; ok {
			return nil, fmt.Errorf("duplicate profile name %q: defined in both %s and %s",
				profile.Name, path, existing.Name)
		}
		profiles[profile.Name] = profile
	}

	return profiles, nil
}
