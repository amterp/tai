package install

import (
	"fmt"
	"strings"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
)

// knownAssetTypes is the set of asset types that validation recognizes.
// Note: "file" type assets skip frontmatter validation entirely.
var knownAssetTypes = map[string]bool{
	config.AssetTypePersona:  true,
	config.AssetTypeDelegate: true,
	config.AssetTypeSkill:    true,
	config.AssetTypeFile:     true,
}

// validateAssetMetadata checks that parsed frontmatter satisfies tai's
// requirements for the given asset type:
//   - the asset type is recognized
//   - name is non-empty for types that require it (personas, delegates, skills)
//
// Unknown fields are silently ignored to support external sources with their
// own metadata. Nil metadata is valid for file assets (they don't use frontmatter)
// but an error for other types.
func validateAssetMetadata(metadata map[string]string, assetType string) error {
	if !knownAssetTypes[assetType] {
		return fmt.Errorf("unknown asset type %q", assetType)
	}

	// File assets don't use frontmatter - they're copied as-is
	if assetType == config.AssetTypeFile {
		return nil
	}

	if metadata == nil {
		return fmt.Errorf("missing required field %q for %s", frontmatter.FieldName, assetType)
	}

	// Check required fields per type
	switch assetType {
	case config.AssetTypePersona, config.AssetTypeDelegate, config.AssetTypeSkill:
		if strings.TrimSpace(metadata[frontmatter.FieldName]) == "" {
			return fmt.Errorf("missing required field %q for %s", frontmatter.FieldName, assetType)
		}
	}

	return nil
}

