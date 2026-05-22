package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/pathutil"
)

const (
	// commandStderrTailBytes caps how much stderr is shown in error messages.
	// The full stderr is captured up to commandOutputCapBytes; only the tail is
	// included in errors to keep them readable. Note: scripts that print
	// secrets to stderr right before failing may leak those secrets into tai's
	// output - the documented mitigation is to use env vars for credentials
	// and not echo them.
	commandStderrTailBytes = 4 * 1024        // 4 KB shown in errors
	commandOutputCapBytes  = 1 * 1024 * 1024 // 1 MB cap on each of stdout/stderr capture
)

// resolveCommandInclude runs a command-based include. The command is executed
// with a tai-managed staging directory exposed via TAI_DEST_DIR. On success,
// the staging directory's contents are atomically swapped into the include's
// final destination. On failure, the staging directory is preserved for
// debugging and its path is included in the returned error.
func (r *IncludeResolver) resolveCommandInclude(inc config.Include, targetSkillPath string) error {
	if len(inc.Command) == 0 {
		return fmt.Errorf("command is empty (validate should have caught this)")
	}

	// Compute the final destination first so we fail fast on path-traversal
	// before spending time running the script.
	destPath, err := r.computeCommandDestPath(inc.Dest, targetSkillPath)
	if err != nil {
		return err
	}

	staging, err := os.MkdirTemp("", "tai-fetch-*")
	if err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	// Staging is removed on success after the swap; on failure we leave it for
	// debugging and surface the path in the error.

	timeout := inc.ResolvedTimeout()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, inc.Command[0], inc.Command[1:]...)
	cmd.Env = r.commandEnv(staging)
	// Put the child in its own process group (POSIX) so that when the context
	// is cancelled (e.g. on timeout), we can signal the whole group and clean
	// up any grandchildren (e.g. `sh -c "curl ..."` where curl is the long
	// thing). No-op on platforms that don't support process groups.
	applyProcessGroup(cmd)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &cappedWriter{W: &stdoutBuf, Cap: commandOutputCapBytes}
	cmd.Stderr = &cappedWriter{W: &stderrBuf, Cap: commandOutputCapBytes}

	runErr := cmd.Run()
	// A timeout is only a *timeout* if the run actually failed because of it.
	// Otherwise the script may have completed in the same tick the deadline
	// fired, which would produce a misleading "timed out" error for a
	// successful run.
	if runErr != nil && ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command %v timed out after %s; increase 'timeout' in your include if the script needs longer (staging: %s)\nstderr (tail):\n%s",
			redactArgv(inc.Command), timeout, staging, tail(stderrBuf.Bytes(), commandStderrTailBytes))
	}
	if runErr != nil {
		// For ExitError we already extract the exit code into the message;
		// don't also %w it or the formatted output renders the same code twice
		// ("(exit 1): exit status 1").
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return fmt.Errorf("command %v exited %d (staging: %s)\nstderr (tail):\n%s",
				redactArgv(inc.Command), exitErr.ExitCode(), staging, tail(stderrBuf.Bytes(), commandStderrTailBytes))
		}
		// Non-exit failures (spawn errors, etc.) - keep the wrapped cause.
		return fmt.Errorf("command %v failed to run (staging: %s): %w\nstderr (tail):\n%s",
			redactArgv(inc.Command), staging, runErr, tail(stderrBuf.Bytes(), commandStderrTailBytes))
	}

	// Verify the script actually produced output. Silent zero-output success
	// is almost always a bug; surface it loudly with a hint about the
	// contract.
	empty, err := isDirEmpty(staging)
	if err != nil {
		return fmt.Errorf("inspecting staging dir %q: %w", staging, err)
	}
	if empty {
		return fmt.Errorf("command %v exited 0 but wrote no files; scripts must write into $TAI_DEST_DIR (= %s)",
			redactArgv(inc.Command), staging)
	}

	if err := swapInto(staging, destPath); err != nil {
		return fmt.Errorf("swapping staging into %q (staging: %s): %w", destPath, staging, err)
	}
	return nil
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

// commandEnv builds the env vars passed to the include's command. The user's
// environment is inherited so PATH, secrets in env (e.g. $GITLAB_TOKEN), and
// HOME are all available. Any pre-existing TAI_* variables in the inherited
// env are stripped so tai's authoritative values are unambiguously visible
// regardless of runtime-specific env-deduplication rules.
//
// All TAI_* variables are always set (possibly to empty string) so scripts
// running under `set -u` can reference them without surprise.
func (r *IncludeResolver) commandEnv(stagingDir string) []string {
	inherited := os.Environ()
	env := make([]string, 0, len(inherited)+6)
	for _, kv := range inherited {
		if strings.HasPrefix(kv, "TAI_") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"TAI_DEST_DIR="+stagingDir,
		"TAI_ASSET_TYPE="+r.cmdCtx.AssetType,
		"TAI_ASSET_NAME="+r.cmdCtx.AssetName,
		"TAI_TARGET="+r.cmdCtx.TargetName,
		"TAI_MANIFEST_DIR="+r.cmdCtx.ManifestDir,
		"TAI_VERSION="+r.cmdCtx.Version,
	)
	return env
}

// swapInto moves staging into destPath atomically when possible. If destPath
// exists, it is removed first (matching the wipe-then-replace behavior of
// other include resolvers). os.Rename is atomic on POSIX within a single
// filesystem; on EXDEV we fall back to recursive copy + remove.
func swapInto(staging, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating dest parent: %w", err)
	}
	if _, err := os.Lstat(destPath); err == nil {
		if err := os.RemoveAll(destPath); err != nil {
			return fmt.Errorf("removing existing dest: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat dest: %w", err)
	}

	if err := os.Rename(staging, destPath); err == nil {
		return nil
	} else if !isCrossDeviceErr(err) {
		return fmt.Errorf("rename: %w", err)
	}

	// Cross-filesystem fallback: copy then remove.
	if err := copyDirectory(staging, destPath); err != nil {
		return fmt.Errorf("cross-fs copy: %w", err)
	}
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("removing staging after copy: %w", err)
	}
	return nil
}

func isCrossDeviceErr(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		if errno, ok := linkErr.Err.(syscall.Errno); ok {
			return errno == syscall.EXDEV
		}
	}
	return false
}

func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// redactArgv returns a copy of argv suitable for logging. Today this is just
// the argv verbatim - we don't try to guess which arguments are secrets,
// because the heuristics are wrong more often than they're right. Docs warn
// users to keep credentials in env vars (which we don't log) rather than in
// argv. This indirection is here so a future opt-in redaction can be slotted
// in without touching every error site.
func redactArgv(argv []string) []string {
	return argv
}

// cappedWriter wraps an io.Writer with a maximum total byte cap. Writes past
// the cap are silently dropped (the underlying writer receives nothing).
// Used to bound memory usage when capturing command stdout/stderr.
//
// All Write calls return (len(p), nil) regardless of underlying behavior:
// the writer's contract is "the process keeps running and we don't drop the
// frame" - propagating short writes or underlying errors would risk stalling
// or breaking the child process for no benefit (we just want the bytes for
// the error tail, we don't care if the underlying buffer dropped some).
type cappedWriter struct {
	W       io.Writer
	Cap     int
	written int
}

func (cw *cappedWriter) Write(p []byte) (int, error) {
	if cw.written < cw.Cap {
		remaining := cw.Cap - cw.written
		writable := p
		if len(p) > remaining {
			writable = p[:remaining]
		}
		n, _ := cw.W.Write(writable)
		cw.written += n
	}
	// Pretend we wrote everything to avoid EPIPE / short-write retries.
	return len(p), nil
}

// tail returns the last `max` bytes of b (or all of b if shorter). When
// truncated, it prefixes the result with an ellipsis marker so readers can
// tell output was cut.
func tail(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return "...(truncated)...\n" + string(b[len(b)-max:])
}
