package install

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/fetch"
	"github.com/amterp/tai/internal/pathutil"
	"github.com/amterp/tai/internal/source"
)

// IncludeResolver handles resolving skill includes from sources, URLs, and commands.
type IncludeResolver struct {
	sources    map[string]*source.LoadedSource
	httpClient *http.Client

	// cmdCtx supplies the TAI_* env vars passed to command-based includes.
	// Zero values are tolerated; the corresponding env vars are simply unset.
	cmdCtx CommandContext
}

// CommandContext carries the per-skill metadata that gets exposed to
// command-based include scripts via TAI_* environment variables.
type CommandContext struct {
	AssetType   string // e.g. "skill"
	AssetName   string // the asset's name
	TargetName  string // e.g. "claude", "codex"
	ManifestDir string // absolute path to the source skill directory
	Version     string // tai's version string
}

// NewIncludeResolver creates a new include resolver.
func NewIncludeResolver(sources map[string]*source.LoadedSource, httpClient *http.Client) *IncludeResolver {
	return &IncludeResolver{
		sources:    sources,
		httpClient: httpClient,
	}
}

// SetCommandContext configures the metadata exposed to command-based includes.
// Must be called before ResolveIncludes when the manifest contains command
// entries; otherwise the TAI_* env vars will be unset (harmless but less
// useful for scripts that rely on them).
func (r *IncludeResolver) SetCommandContext(ctx CommandContext) {
	r.cmdCtx = ctx
}

// ResolveIncludes processes all includes for a skill, copying files to the target skill path.
func (r *IncludeResolver) ResolveIncludes(manifest *config.SkillManifest, targetSkillPath string) error {
	for i, inc := range manifest.Include {
		var err error
		switch {
		case inc.IsCommandBased():
			err = r.resolveCommandInclude(inc, targetSkillPath)
		case inc.IsURLBased():
			err = r.resolveURLInclude(inc, targetSkillPath)
		default:
			err = r.resolveSourceInclude(inc, targetSkillPath)
		}
		if err != nil {
			return fmt.Errorf("include[%d]: %w", i, err)
		}
	}
	return nil
}

// resolveSourceInclude copies a file or directory from a named source.
func (r *IncludeResolver) resolveSourceInclude(inc config.Include, targetSkillPath string) error {
	src, ok := r.sources[inc.From]
	if !ok {
		return fmt.Errorf("source %q not found", inc.From)
	}

	// ContentPath, not ResolvedPath: an include must pull the same patched
	// bytes the rest of the install sees, or one source file would appear two
	// different ways in a single install.
	srcRoot := src.ContentPath()
	srcPath := filepath.Join(srcRoot, inc.Path)

	// Security: Validate source path doesn't escape source directory (path traversal protection)
	if !pathutil.IsContainedIn(srcPath, srcRoot) {
		return fmt.Errorf("path %q escapes source directory (path traversal not allowed)", inc.Path)
	}

	// Verify source path exists
	info, err := os.Lstat(srcPath) // Use Lstat to detect symlinks
	if os.IsNotExist(err) {
		return fmt.Errorf("path %q not found in source %q", inc.Path, inc.From)
	}
	if err != nil {
		return fmt.Errorf("accessing %q in source %q: %w", inc.Path, inc.From, err)
	}

	// Security: Reject symlinks to prevent escaping source directory
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path %q in source %q is a symlink (not allowed for security)", inc.Path, inc.From)
	}

	destPath, err := r.computeDestPath(inc.Dest, inc.Path, "", targetSkillPath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	if info.IsDir() {
		return pathutil.CopyDir(srcPath, destPath)
	}
	return pathutil.CopyFile(srcPath, destPath)
}

// resolveURLInclude fetches a file from a URL and writes it to the destination.
func (r *IncludeResolver) resolveURLInclude(inc config.Include, targetSkillPath string) error {
	if _, err := fetch.ParseURL(inc.URL); err != nil {
		return err
	}

	destPath, err := r.computeDestPath(inc.Dest, "", inc.URL, targetSkillPath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	return fetch.ToFile(r.httpClient, inc.URL, destPath, "")
}

// computeDestPath determines the final destination path.
// If dest ends with "/", it's a directory and we preserve the original filename.
// Otherwise, dest is the exact target path.
// Returns an error if the destination would escape the target skill path.
func (r *IncludeResolver) computeDestPath(dest, srcPath, srcURL, targetSkillPath string) (string, error) {
	var destPath string

	if strings.HasSuffix(dest, "/") {
		// Directory mode - preserve original filename
		filename, err := extractFilename(srcPath, srcURL)
		if err != nil {
			return "", err
		}
		destPath = filepath.Join(targetSkillPath, dest, filename)
	} else {
		destPath = filepath.Join(targetSkillPath, dest)
	}

	// Security: Validate destination doesn't escape skill directory (path traversal protection)
	if !pathutil.IsContainedIn(destPath, targetSkillPath) {
		return "", fmt.Errorf("destination %q escapes skill directory (path traversal not allowed)", dest)
	}

	return destPath, nil
}

// extractFilename extracts a valid filename from either a source path or URL.
func extractFilename(srcPath, srcURL string) (string, error) {
	var filename string

	if srcURL != "" {
		// Extract filename from URL
		parsed, err := url.Parse(srcURL)
		if err != nil {
			return "", fmt.Errorf("cannot parse URL %q: %w", srcURL, err)
		}
		filename = path.Base(parsed.Path)
	} else {
		filename = filepath.Base(srcPath)
	}

	// Validate filename is usable
	if filename == "" || filename == "." || filename == "/" {
		if srcURL != "" {
			return "", fmt.Errorf("cannot determine filename from URL %q", srcURL)
		}
		return "", fmt.Errorf("cannot determine filename from path %q", srcPath)
	}

	return filename, nil
}
