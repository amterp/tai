package install

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/cmdexec"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/pathutil"
)

// resolveCommandInclude runs a command-based include. The command is executed
// with a tai-managed staging directory exposed via TAI_DEST_DIR. On success,
// the staging directory's contents are atomically swapped into the include's
// final destination. On failure, the staging directory is preserved for
// debugging and its path is included in the returned error.
//
// A dest at the skill root is merged rather than swapped: each file or
// directory the command wrote replaces the same path in the skill, and the
// rest of the skill stays. Swapping would replace the whole skill with the
// command's output; merging is what lets a command produce SKILL.md itself.
//
// It returns the skill-relative paths it wrote.
func (r *IncludeResolver) resolveCommandInclude(inc config.Include, targetSkillPath string) ([]string, error) {
	// Compute the final destination first so we fail fast on path-traversal
	// before spending time running the script.
	destPath, err := r.computeCommandDestPath(inc.Dest, targetSkillPath)
	if err != nil {
		return nil, err
	}

	spec := cmdexec.Spec{
		Argv:    inc.Command,
		Timeout: inc.ResolvedTimeout(),
		Env: map[string]string{
			"TAI_ASSET_TYPE":   r.cmdCtx.AssetType,
			"TAI_ASSET_NAME":   r.cmdCtx.AssetName,
			"TAI_TARGET":       r.cmdCtx.TargetName,
			"TAI_MANIFEST_DIR": r.cmdCtx.ManifestDir,
			"TAI_VERSION":      r.cmdCtx.Version,
		},
	}
	if destPath == targetSkillPath {
		return cmdexec.MergeInto(spec, destPath)
	}
	rel, _ := filepath.Rel(targetSkillPath, destPath)
	return []string{rel}, cmdexec.FetchInto(spec, destPath, "")
}

// computeCommandDestPath resolves the final dest path for a command include,
// applying the same path-traversal guards as the URL/source paths. A dest at
// the skill root (".", "./", "/") resolves to targetSkillPath itself.
func (r *IncludeResolver) computeCommandDestPath(dest, targetSkillPath string) (string, error) {
	if dest == "" {
		return "", fmt.Errorf("include with 'command' requires 'dest'")
	}
	destPath := filepath.Join(targetSkillPath, strings.TrimRight(dest, "/"))
	if !pathutil.IsContainedIn(destPath, targetSkillPath) {
		return "", fmt.Errorf("destination %q escapes skill directory (path traversal not allowed)", dest)
	}
	return destPath, nil
}
