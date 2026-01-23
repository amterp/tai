package install

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/pathutil"
	"github.com/amterp/tai/internal/source"
)

const (
	// DefaultHTTPTimeout is the default timeout for HTTP requests when fetching URL includes.
	DefaultHTTPTimeout = 30 * time.Second

	// MaxIncludeSize is the maximum size in bytes for a single URL include (10MB).
	MaxIncludeSize = 10 * 1024 * 1024
)

// IncludeResolver handles resolving skill includes from sources and URLs.
type IncludeResolver struct {
	sources    map[string]*source.LoadedSource
	httpClient *http.Client
}

// NewIncludeResolver creates a new include resolver.
func NewIncludeResolver(sources map[string]*source.LoadedSource, httpClient *http.Client) *IncludeResolver {
	return &IncludeResolver{
		sources:    sources,
		httpClient: httpClient,
	}
}

// ResolveIncludes processes all includes for a skill, copying files to the target skill path.
func (r *IncludeResolver) ResolveIncludes(manifest *config.SkillManifest, targetSkillPath string) error {
	for i, inc := range manifest.Include {
		var err error
		if inc.IsURLBased() {
			err = r.resolveURLInclude(inc, targetSkillPath)
		} else {
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

	srcPath := filepath.Join(src.ResolvedPath, inc.Path)

	// Security: Validate source path doesn't escape source directory (path traversal protection)
	if !pathutil.IsContainedIn(srcPath, src.ResolvedPath) {
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
		return r.copyDirectory(srcPath, destPath)
	}
	return copyFile(srcPath, destPath)
}

// resolveURLInclude fetches a file from a URL and writes it to the destination.
func (r *IncludeResolver) resolveURLInclude(inc config.Include, targetSkillPath string) error {
	// Security: Validate URL scheme
	parsed, err := url.Parse(inc.URL)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", inc.URL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("URL %q has invalid scheme %q (must be http or https)", inc.URL, parsed.Scheme)
	}

	resp, err := r.httpClient.Get(inc.URL)
	if err != nil {
		return fmt.Errorf("fetching %q: %w", inc.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %q: HTTP %d %s", inc.URL, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	destPath, err := r.computeDestPath(inc.Dest, "", inc.URL, targetSkillPath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating destination file %q: %w", destPath, err)
	}
	defer func() { _ = destFile.Close() }()

	// Security: Limit download size to prevent resource exhaustion
	limitedReader := io.LimitReader(resp.Body, MaxIncludeSize+1)
	written, err := io.Copy(destFile, limitedReader)
	if err != nil {
		return fmt.Errorf("writing to %q: %w", destPath, err)
	}
	if written > MaxIncludeSize {
		_ = os.Remove(destPath) // Clean up oversized file
		return fmt.Errorf("URL %q exceeds maximum size of %d bytes", inc.URL, MaxIncludeSize)
	}

	return nil
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

// copyDirectory copies a directory recursively.
// Rejects symlinks for security.
func (r *IncludeResolver) copyDirectory(src, dst string) error {
	// Remove existing destination if it exists
	_ = os.RemoveAll(dst)

	return filepath.WalkDir(src, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Security: Skip symlinks
		if d.Type()&os.ModeSymlink != 0 {
			return nil // Skip symlinks silently in directory traversal
		}

		relPath, err := filepath.Rel(src, srcPath)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return mkdirPreservingMode(dstPath, d)
		}

		return copyFile(srcPath, dstPath)
	})
}

// DefaultHTTPClient returns an HTTP client with sensible defaults for fetching includes.
func DefaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: DefaultHTTPTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Security: Limit redirects and validate scheme after redirect
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to non-HTTP scheme %q not allowed", req.URL.Scheme)
			}
			return nil
		},
	}
}
