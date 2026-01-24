package install

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
)

// allowedFields defines which frontmatter fields are permitted per asset type,
// beyond tai-schema-version (which is always allowed and required).
var allowedFields = map[string][]string{
	config.AssetTypePersona:  {frontmatter.FieldName, frontmatter.FieldDescription},
	config.AssetTypeDelegate: {frontmatter.FieldName, frontmatter.FieldDescription, frontmatter.FieldTools},
	config.AssetTypeSkill:    {frontmatter.FieldName, frontmatter.FieldDescription, frontmatter.FieldTools},
	config.AssetTypeDoc:      {frontmatter.FieldName, frontmatter.FieldDescription},
}

// validateAssetMetadata checks that parsed frontmatter satisfies tai's schema
// requirements for the given asset type. It verifies:
//   - tai-schema-version is present and supported
//   - name is non-empty for asset types that require it (personas, delegates, skills)
//   - no unrecognized fields are present
//   - the asset type itself is recognized
func validateAssetMetadata(metadata map[string]string, assetType string) error {
	if metadata == nil {
		return fmt.Errorf("missing frontmatter (%s: %s required)",
			frontmatter.FieldTaiSchemaVersion, frontmatter.CurrentTaiSchemaVersion)
	}

	version, ok := metadata[frontmatter.FieldTaiSchemaVersion]
	if !ok {
		return fmt.Errorf("missing required field %q", frontmatter.FieldTaiSchemaVersion)
	}
	if version != frontmatter.CurrentTaiSchemaVersion {
		return fmt.Errorf("unsupported %s: %q (only %q is supported)",
			frontmatter.FieldTaiSchemaVersion, version, frontmatter.CurrentTaiSchemaVersion)
	}

	allowed, ok := allowedFields[assetType]
	if !ok {
		return fmt.Errorf("unknown asset type %q", assetType)
	}

	// Check required fields
	switch assetType {
	case config.AssetTypePersona, config.AssetTypeDelegate, config.AssetTypeSkill:
		if strings.TrimSpace(metadata[frontmatter.FieldName]) == "" {
			return fmt.Errorf("missing required field %q for %s", frontmatter.FieldName, assetType)
		}
	}

	// Reject unrecognized fields
	allowedSet := make(map[string]bool, len(allowed)+1)
	allowedSet[frontmatter.FieldTaiSchemaVersion] = true
	for _, f := range allowed {
		allowedSet[f] = true
	}

	for field := range metadata {
		if !allowedSet[field] {
			return fmt.Errorf("unknown field %q for %s (allowed: %s)",
				field, assetType, formatAllowedFields(allowed))
		}
	}

	return nil
}

// formatAllowedFields produces a human-readable list of allowed field names.
func formatAllowedFields(fields []string) string {
	sorted := make([]string, len(fields))
	copy(sorted, fields)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}
