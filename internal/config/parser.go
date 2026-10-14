package config

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/amterp/tai/internal/fetch"
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
		// Add implicit local source at the beginning (lowest priority).
		// It roots at the config repo itself, not at ./assets: a source is a
		// directory root, and the repo's bundles/ has to be reachable from it.
		implicitLocal := Source{
			Name: "local",
			Type: "local",
			Path: ".",
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
		// The name doubles as a cache directory name for command sources;
		// enforce it's a safe single path segment for all types so changing
		// a source's type never introduces traversal.
		if strings.ContainsAny(s.Name, `/\`) || s.Name == "." || s.Name == ".." {
			return fmt.Errorf("source name %q must be a plain name (no path separators or '.'/'..')", s.Name)
		}
		if seen[s.Name] {
			return fmt.Errorf("duplicate source name: %s", s.Name)
		}
		seen[s.Name] = true

		// Normalize type
		if s.Type == "" {
			config.Sources[i].Type = "local"
		}

		switch config.Sources[i].Type {
		case "tai-repo":
			// Removed: it appended "/assets" to the path, which put a source's
			// bundles/ outside anything tai could address. Sources are plain
			// directory roots now, so the append is the caller's to write.
			return fmt.Errorf("source %q: type 'tai-repo' has been removed. "+
				"Sources are plain directory roots: drop the 'type' line, keep 'path' as-is, "+
				"and prefix this source's asset references with 'assets/' "+
				"(e.g. '%s:skills/foo' becomes '%s:assets/skills/foo')", s.Name, s.Name, s.Name)
		case "local":
			if s.Path == "" {
				return fmt.Errorf("source %q: missing path", s.Name)
			}
			if len(s.Command) > 0 {
				return fmt.Errorf("source %q: 'command' is only valid with type 'command'", s.Name)
			}
			if s.URL != "" {
				return fmt.Errorf("source %q: 'url' is only valid with type 'url'", s.Name)
			}
			if s.Filename != "" {
				return fmt.Errorf("source %q: 'filename' is only valid with type 'url'", s.Name)
			}
			if s.Timeout != "" {
				return fmt.Errorf("source %q: 'timeout' is only valid with type 'command' or 'url'", s.Name)
			}
		case SourceTypeCommand:
			if len(s.Command) == 0 {
				return fmt.Errorf("source %q: type 'command' requires 'command'", s.Name)
			}
			if err := validateCommandArgv(s.Command, "command"); err != nil {
				return fmt.Errorf("source %q: %w", s.Name, err)
			}
			if s.Path != "" {
				return fmt.Errorf("source %q: 'path' is not valid with type 'command'", s.Name)
			}
			if s.URL != "" {
				return fmt.Errorf("source %q: 'url' is only valid with type 'url'", s.Name)
			}
			if s.Filename != "" {
				return fmt.Errorf("source %q: 'filename' is only valid with type 'url'", s.Name)
			}
			if err := validateTimeoutString(s.Timeout, "timeout"); err != nil {
				return fmt.Errorf("source %q: %w", s.Name, err)
			}
		case SourceTypeURL:
			if s.URL == "" {
				return fmt.Errorf("source %q: type 'url' requires 'url'", s.Name)
			}
			// Checked here rather than at fetch time so a typo'd URL is a
			// config error, not a failure part-way through an install.
			if _, err := fetch.ParseURL(s.URL); err != nil {
				return fmt.Errorf("source %q: %w", s.Name, err)
			}
			if s.Path != "" {
				return fmt.Errorf("source %q: 'path' is not valid with type 'url'", s.Name)
			}
			if len(s.Command) > 0 {
				return fmt.Errorf("source %q: 'command' is only valid with type 'command'", s.Name)
			}
			// The filename becomes a path segment inside the source's cache
			// dir and the `path:` includes address it by, so it has to be a
			// plain name.
			if s.Filename != "" && (strings.ContainsAny(s.Filename, `/\`) || s.Filename == "." || s.Filename == "..") {
				return fmt.Errorf("source %q: 'filename' must be a plain name (no path separators or '.'/'..'): %q", s.Name, s.Filename)
			}
			if s.ResolvedFilename() == "" {
				return fmt.Errorf("source %q: cannot derive a filename from url %q; set 'filename:'", s.Name, s.URL)
			}
			if err := validateTimeoutString(s.Timeout, "timeout"); err != nil {
				return fmt.Errorf("source %q: %w", s.Name, err)
			}
		default:
			return fmt.Errorf("source %q: invalid type %q (expected 'local', 'command', or 'url')", s.Name, s.Type)
		}

		// Patches and exclusions apply to every source type.
		if s.Patches != "" {
			if err := validateRelPath(s.Patches, "patches"); err != nil {
				return fmt.Errorf("source %q: %w", s.Name, err)
			}
		}
		for j, pattern := range s.Exclude {
			if strings.TrimSpace(pattern) == "" {
				return fmt.Errorf("source %q: exclude[%d] is empty", s.Name, j)
			}
			if err := validateRelPath(pattern, "exclude pattern"); err != nil {
				return fmt.Errorf("source %q: %w", s.Name, err)
			}
			if err := validateExcludePattern(pattern); err != nil {
				return fmt.Errorf("source %q: exclude[%d]: %w", s.Name, j, err)
			}
		}
	}

	return nil
}

// validateExcludePattern rejects patterns that can never match anything.
//
// Both failures here are silent by nature: path.Match gives `**` no special
// meaning and reports a malformed pattern as an error IsExcluded cannot act on,
// so either one leaves the file quietly included - the opposite of what was
// asked for, with nothing to notice. Catching them at parse time turns a file
// that shouldn't have shipped into a config error.
func validateExcludePattern(pattern string) error {
	// A trailing `/**` is sugar IsExcluded strips; anything further in is a
	// cross-segment wildcard that path.Match will not honor.
	if strings.Contains(strings.TrimSuffix(pathpkg.Clean(filepath.ToSlash(pattern)), "/**"), "**") {
		return fmt.Errorf("`**` only spans directories at the end of a pattern: %q. "+
			"Wildcards do not cross `/`, but excluding a directory already excludes everything under it - "+
			"use `a/b` to drop that whole subtree", pattern)
	}
	if _, err := pathpkg.Match(pattern, "probe"); err != nil {
		return fmt.Errorf("malformed pattern %q: %w", pattern, err)
	}
	return nil
}

// validateRelPath rejects absolute paths and traversal, mirroring
// validateDestPath. Used for source-declared patch dirs and exclude patterns,
// both of which must stay inside their repo or source.
func validateRelPath(p, field string) error {
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("%s cannot be an absolute path: %q", field, p)
	}
	for _, segment := range strings.Split(filepath.ToSlash(p), "/") {
		if segment == ".." {
			return fmt.Errorf("%s cannot contain path traversal (..): %q", field, p)
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

	for i, entry := range bundle.Bundles {
		if err := validateBundleEntry(entry, i); err != nil {
			return fmt.Errorf("bundle %q: %w", bundle.Name, err)
		}
	}

	for i, entry := range bundle.Assets {
		if err := validateAssetEntry(entry, i); err != nil {
			return fmt.Errorf("bundle %q: %w", bundle.Name, err)
		}
	}

	return nil
}

// validateBundleEntry validates a single entry in a `bundles:` list.
func validateBundleEntry(entry BundleEntry, index int) error {
	if strings.TrimSpace(entry.Name) == "" {
		return fmt.Errorf("bundle[%d]: missing name", index)
	}

	sourceName, bundleName, qualified := ParseBundleReference(entry.Name)
	if qualified && sourceName == "" {
		return fmt.Errorf("bundle[%d]: reference %q has an empty source name; write 'source:bundle' or a bare bundle name", index, entry.Name)
	}
	if bundleName == "" {
		return fmt.Errorf("bundle[%d]: reference %q names a source but no bundle", index, entry.Name)
	}

	for j, pattern := range entry.Exclude {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("bundle[%d]: exclude[%d] is empty", index, j)
		}
		if err := validateRelPath(pattern, "exclude pattern"); err != nil {
			return fmt.Errorf("bundle[%d]: %w", index, err)
		}
		if err := validateExcludePattern(pattern); err != nil {
			return fmt.Errorf("bundle[%d]: exclude[%d]: %w", index, j, err)
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
	if profile.Scope != "" && !IsValidScope(profile.Scope) {
		return fmt.Errorf("profile %q: invalid scope %q; must be one of: %s",
			profile.Name, profile.Scope, strings.Join(ValidScopes(), ", "))
	}

	for i, entry := range profile.Bundles {
		if err := validateBundleEntry(entry, i); err != nil {
			return fmt.Errorf("profile %q: %w", profile.Name, err)
		}
	}

	for i, entry := range profile.Assets {
		if err := validateAssetEntry(entry, i); err != nil {
			return fmt.Errorf("profile %q: %w", profile.Name, err)
		}
	}

	if len(profile.PostInstall) > 0 {
		if err := validatePostInstallSteps(profile.PostInstall, "post_install"); err != nil {
			return fmt.Errorf("profile %q: %w", profile.Name, err)
		}
	}
	if err := validateTimeoutString(profile.PostInstallTimeout, "post_install_timeout"); err != nil {
		return fmt.Errorf("profile %q: %w", profile.Name, err)
	}
	if profile.PostInstallTimeout != "" && len(profile.PostInstall) == 0 {
		return fmt.Errorf("profile %q: 'post_install_timeout' is only valid with 'post_install'", profile.Name)
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

	if entry.Scope != "" {
		if !IsConfigAssetType(entry.Type) {
			return fmt.Errorf("asset[%d]: 'scope' is only valid for config asset types, not %q", index, entry.Type)
		}
		if !IsValidScope(entry.Scope) {
			return fmt.Errorf("asset[%d]: invalid scope %q; must be one of: %s",
				index, entry.Scope, strings.Join(ValidScopes(), ", "))
		}
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

// ParseBundleReference parses a bundle reference into source and bundle name.
// Format: "source:name" or just "name".
//
// Unlike ParseAssetReference there is no default source, hence the qualified
// return: an unprefixed name resolves against whichever repo the referencing
// bundle itself came from, and only the resolver knows what that is.
func ParseBundleReference(ref string) (sourceName, bundleName string, qualified bool) {
	if idx := strings.Index(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:], true
	}
	return "", ref, false
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

// ParseStatusLineConfig parses status line config YAML.
//
// A config carrying a `level` field is tai format and is returned parsed, for
// the adapter to translate. Without one it is native format: the raw bytes come
// back instead, to be passed through to the target untouched.
//
// It takes bytes rather than a path deliberately. Config assets are templated
// before parsing, and reading the file here would both skip templating and
// bypass a source's patched view - the silent-wrong-content failure that has
// bitten this project twice.
func ParseStatusLineConfig(data []byte) (*StatusLineConfig, []byte, error) {
	var cfg StatusLineConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parsing status line config: %w", err)
	}

	if !cfg.IsTaiFormat() {
		return nil, data, nil
	}

	return &cfg, nil, nil
}

// ParseHookConfig parses hook asset YAML.
//
// Like ParseStatusLineConfig it takes bytes, not a path: config assets are
// templated before parsing, and reading the file here would skip templating
// and bypass a source's patched view.
func ParseHookConfig(data []byte) (*HookConfig, error) {
	var cfg HookConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing hook config: %w", err)
	}
	return &cfg, nil
}

// ParseMCPConfig parses MCP server asset YAML.
func ParseMCPConfig(data []byte) (*MCPConfig, error) {
	var cfg MCPConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing mcp config: %w", err)
	}
	return &cfg, nil
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
