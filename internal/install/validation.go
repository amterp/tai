package install

import (
	"fmt"
	"slices"
	"strings"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
)

// validateStatusLineLevelValid checks if the level is valid (used by validateStatusLineConfig).
func validateStatusLineLevelValid(level string) bool {
	validLevels := []string{
		config.StatusLineLevelMinimal,
		config.StatusLineLevelStandard,
		config.StatusLineLevelDetailed,
		config.StatusLineLevelHidden,
		config.StatusLineLevelCustom,
	}
	return slices.Contains(validLevels, level)
}

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

// validateStatusLineConfig validates a tai-format status line configuration.
func validateStatusLineConfig(cfg *config.StatusLineConfig) error {
	if cfg == nil {
		return nil
	}

	// Validate level
	if !validateStatusLineLevelValid(cfg.Level) {
		validLevels := []string{
			config.StatusLineLevelMinimal,
			config.StatusLineLevelStandard,
			config.StatusLineLevelDetailed,
			config.StatusLineLevelHidden,
			config.StatusLineLevelCustom,
		}
		return fmt.Errorf("invalid level %q; must be one of: %s",
			cfg.Level, strings.Join(validLevels, ", "))
	}

	// Level-specific validation
	switch cfg.Level {
	case config.StatusLineLevelCustom:
		if cfg.Command == "" {
			return fmt.Errorf("'command' is required when level is 'custom'")
		}
		if len(cfg.Show) > 0 {
			return fmt.Errorf("'show' is not allowed when level is 'custom'")
		}
	case config.StatusLineLevelHidden:
		if len(cfg.Show) > 0 {
			return fmt.Errorf("'show' is not allowed when level is 'hidden'")
		}
		if cfg.Command != "" {
			return fmt.Errorf("'command' is not allowed when level is 'hidden'")
		}
	default: // minimal, standard, detailed
		if cfg.Command != "" {
			return fmt.Errorf("'command' is only allowed when level is 'custom'")
		}
	}

	// Note: Unknown show elements are silently ignored for forward compatibility.
	// This allows configs to use newer elements without failing on older tai versions.

	return nil
}
