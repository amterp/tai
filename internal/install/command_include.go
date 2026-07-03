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
func (r *IncludeResolver) resolveCommandInclude(inc config.Include, targetSkillPath string) error {
	// Compute the final destination first so we fail fast on path-traversal
	// before spending time running the script.
	destPath, err := r.computeCommandDestPath(inc.Dest, targetSkillPath)
	if err != nil {
		return err
	}

	return cmdexec.FetchInto(cmdexec.Spec{
		Argv:    inc.Command,
		Timeout: inc.ResolvedTimeout(),
		Env: map[string]string{
			"TAI_ASSET_TYPE":   r.cmdCtx.AssetType,
			"TAI_ASSET_NAME":   r.cmdCtx.AssetName,
			"TAI_TARGET":       r.cmdCtx.TargetName,
			"TAI_MANIFEST_DIR": r.cmdCtx.ManifestDir,
			"TAI_VERSION":      r.cmdCtx.Version,
		},
	}, destPath, "")
}

// computeCommandDestPath resolves the final dest path for a command include,
// applying the same path-traversal guards as the URL/source paths. Validate
// already rejects dest values that resolve to the skill root (".", "/", etc.)
// - this is the runtime backstop.
func (r *IncludeResolver) computeCommandDestPath(dest, targetSkillPath string) (string, error) {
	cleaned := strings.TrimRight(dest, "/")
	if cleaned == "" || filepath.Clean(cleaned) == "." {
		return "", fmt.Errorf("dest %q resolves to the skill root; choose a subdirectory (e.g. refs/)", dest)
	}
	destPath := filepath.Join(targetSkillPath, cleaned)
	if !pathutil.IsContainedIn(destPath, targetSkillPath) || destPath == targetSkillPath {
		return "", fmt.Errorf("destination %q escapes skill directory (path traversal not allowed)", dest)
	}
	return destPath, nil
}
