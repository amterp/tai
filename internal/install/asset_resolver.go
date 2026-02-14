package install

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/template"
)

// Ensure AssetContentResolver implements template.ContentResolver.
var _ template.ContentResolver = (*AssetContentResolver)(nil)

// AssetContentResolver resolves asset references to their body content,
// stripping frontmatter. It implements template.ContentResolver.
type AssetContentResolver struct {
	sources map[string]*source.LoadedSource
}

// NewAssetContentResolver creates a new resolver from loaded sources.
func NewAssetContentResolver(sources map[string]*source.LoadedSource) *AssetContentResolver {
	return &AssetContentResolver{sources: sources}
}

// fileShapedAssetTypes are asset types whose content can be included inline.
// Config asset types (statusline, mcp, hook) cannot be included.
var fileShapedAssetTypes = map[string]bool{
	config.AssetTypePersona:  true,
	config.AssetTypeDelegate: true,
	config.AssetTypeSkill:    true,
	config.AssetTypeFile:     true,
}

// ResolveAssetContent resolves an asset reference and returns its body content
// with frontmatter stripped. The ref format is "source:path" or just "path"
// (defaults to "local" source).
func (r *AssetContentResolver) ResolveAssetContent(ref string) ([]byte, error) {
	sourceName, assetPath := config.ParseAssetReference(ref)

	src, ok := r.sources[sourceName]
	if !ok {
		return nil, fmt.Errorf("source %q not found", sourceName)
	}

	// Infer asset type from the first path segment (e.g. "personas/" -> "persona")
	assetType := inferAssetType(assetPath)
	if assetType == "" {
		return nil, fmt.Errorf("cannot infer asset type from path %q (expected personas/, delegates/, skills/, or files/ prefix)", assetPath)
	}

	// Only file-shaped assets can be included
	if !fileShapedAssetTypes[assetType] {
		return nil, fmt.Errorf("cannot include %s asset %q: only file-shaped assets (personas, delegates, skills, files) can be included", assetType, assetPath)
	}

	// Resolve the full filesystem path (includes path traversal check)
	fullPath, ok := src.GetAsset(assetPath)
	if !ok {
		return nil, fmt.Errorf("asset not found: %s:%s", sourceName, assetPath)
	}

	// Read the file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading asset %q: %w", ref, err)
	}

	// Reject binary content - template processing only makes sense for text
	if !utf8.Valid(content) {
		return nil, fmt.Errorf("asset %q contains binary content (not valid UTF-8); only text assets can be included", ref)
	}

	// Parse and strip frontmatter, returning only the body
	parsed, err := frontmatter.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parsing frontmatter for %q: %w", ref, err)
	}

	return parsed.Body, nil
}

// inferAssetType determines the singular asset type from an asset path's
// directory prefix (e.g. "personas/foo.md" -> "persona").
func inferAssetType(assetPath string) string {
	parts := strings.SplitN(assetPath, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	return config.AssetTypeFromDirName(parts[0])
}
