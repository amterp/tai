package install

import (
	"fmt"
	"os"
	"path"
	"unicode/utf8"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/template"
)

// Ensure AssetContentResolver implements template.ContentResolver and VerbatimReporter.
var (
	_ template.ContentResolver  = (*AssetContentResolver)(nil)
	_ template.VerbatimReporter = (*AssetContentResolver)(nil)
)

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

// IsVerbatim reports whether ref names content from a source that declared
// `template: false`.
func (r *AssetContentResolver) IsVerbatim(ref string) bool {
	sourceName, _ := config.ParseAssetReference(ref)
	src, ok := r.sources[sourceName]
	return ok && !src.Templated()
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

	// An unfetched command source would otherwise fall through to a bare
	// "asset not found"; surface the actionable remedy instead.
	if err := src.RequireMaterialized(); err != nil {
		return nil, err
	}

	assetType := inferAssetType(assetPath)
	if assetType == "" {
		return nil, fmt.Errorf("cannot infer asset type from path %q: it must end in <type-dir>/<name>, "+
			"where the type dir is one of personas/, delegates/, skills/, or files/ "+
			"(e.g. assets/personas/senior-swe.md)", assetPath)
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

// inferAssetType determines the singular asset type from the nearest ancestor
// directory that names one (e.g. "assets/personas/foo.md" and "personas/foo.md"
// both give "persona", and "assets/skills/jj/SKILL.md" gives "skill"). Reading
// the first segment instead would tie the type to how deep the source root
// sits, which is not something a reference should encode. Reading only the
// immediate parent would exclude a skill, whose body sits a level below the
// type dir because the skill itself is a directory.
func inferAssetType(assetPath string) string {
	for dir := path.Dir(assetPath); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if base := path.Base(dir); config.IsAssetTypeDirName(base) {
			return config.AssetTypeFromDirName(base)
		}
	}
	return ""
}
