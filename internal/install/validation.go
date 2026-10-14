package install

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/amterp/tai/internal/adapter"
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

// retiredFields were tai frontmatter fields once. They get their own warning
// because the advice for an unrecognized field - move it under native - is
// wrong for a field no target ever read.
var retiredFields = map[string]string{
	"tai-schema-version": "per-file schema versioning was replaced by the repo-level 'version' in tai.yaml",
}

// frontmatterWarnings reports frontmatter that will not reach the installed
// file. Both cases are silent otherwise, and a dropped field looks exactly
// like a field the target simply ignored, so the only way to notice is to
// diff the installed file against its source.
// knownTargets is every target tai has an adapter for; an empty list disables
// the misspelled-target check, since tai then has nothing to check against.
func frontmatterWarnings(parsed *frontmatter.Parsed, assetType, assetName, target string, knownTargets []string) []string {
	var warnings []string

	fields := sortedKeys(parsed.Metadata)
	for field := range parsed.Structured {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		if frontmatter.IsTaiField(field) {
			if _, structured := parsed.Structured[field]; structured {
				warnings = append(warnings, fmt.Sprintf(
					"%s %q: frontmatter field %q is a list or map, but it takes a single value, so it was not installed. "+
						"Write it as one value (a list of tools as \"tools: Read, Grep\").",
					assetType, assetName, field))
			}
			continue
		}
		if reason, retired := retiredFields[field]; retired {
			warnings = append(warnings, fmt.Sprintf(
				"%s %q: frontmatter field %q no longer does anything - %s. Delete it.",
				assetType, assetName, field, reason))
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"%s %q: frontmatter field %q is not a tai field and was not installed - %s will not see it. "+
				"Move it under 'native.%s' to pass it through in %s's own format.",
			assetType, assetName, field, target, target, target))
	}

	if len(knownTargets) == 0 {
		return warnings
	}
	sort.Strings(knownTargets)
	for _, named := range sortedKeys(parsed.Native) {
		if slices.Contains(knownTargets, named) {
			continue
		}
		if _, removed := adapter.RemovedTarget(named); removed {
			warnings = append(warnings, fmt.Sprintf(
				"%s %q: 'native.%s' names a target tai no longer supports, so those fields reach nothing. Delete them.",
				assetType, assetName, named))
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"%s %q: 'native.%s' names a target tai has no adapter for, so those fields reach nothing. "+
				"Known targets are %s - check the spelling.",
			assetType, assetName, named, strings.Join(knownTargets, ", ")))
	}

	return warnings
}

// fileDestWarning flags a file asset whose dest ends in a dot-name such as
// `.claude-plugin`. A dot anywhere in the last segment makes dest a file path,
// so `plugin.json` installs as a file named `.claude-plugin` rather than into
// that directory, and the dry-run line reads the same either way. The rule
// stays, since `.gitignore` and `.envrc` have the same shape and are files;
// the warning only fires when the source has an extension, because renaming
// `plugin.json` to an extensionless dotfile is unlikely. A dest that is the
// source's stem with a leading dot (`envrc.sh` to `.envrc`) is that rename,
// so it does not warn. resolvedDest is the dest after template processing.
func fileDestWarning(asset *config.Asset, resolvedDest string) string {
	if asset.Type != config.AssetTypeFile || resolvedDest == "" || strings.HasSuffix(resolvedDest, "/") {
		return ""
	}
	base := filepath.Base(resolvedDest)
	if len(base) < 2 || base[0] != '.' || strings.Contains(base[1:], ".") {
		return ""
	}
	if !strings.Contains(strings.TrimLeft(asset.Name, "."), ".") {
		return ""
	}
	if base == "."+strings.TrimSuffix(asset.Name, filepath.Ext(asset.Name)) {
		return ""
	}
	return fmt.Sprintf(
		"file %q: dest %q installs it as a file named %q, because a dest whose last segment contains a dot is read as a file path. "+
			"If %s is meant to be a directory, end the dest with a slash: %q.",
		asset.Name, resolvedDest, base, base, asset.Dest+"/")
}

// validateAssetMetadata checks that parsed frontmatter satisfies tai's
// requirements for the given asset type:
//   - the asset type is recognized
//   - name is non-empty for types that require it (personas, delegates, skills)
//
// Unknown fields do not fail the install - external sources carry their own
// metadata, and tai warns rather than refusing. Nil metadata is valid for file
// assets (they don't use frontmatter) but an error for other types.
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
