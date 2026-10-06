// Package cmdexec runs user-declared commands (skill includes, command
// sources, post-install hooks) with shared machinery: timeouts with
// process-group cleanup, capped output capture, an inherited-but-sanitized
// environment, and an optional staging-dir contract for commands that
// produce files.
package cmdexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// StderrTailBytes caps how much stderr is shown in error messages.
	// The full stderr is captured up to OutputCapBytes; only the tail is
	// included in errors to keep them readable. Note: scripts that print
	// secrets to stderr right before failing may leak those secrets into
	// tai's output - the documented mitigation is to use env vars for
	// credentials and not echo them.
	StderrTailBytes = 4 * 1024        // 4 KB shown in errors
	OutputCapBytes  = 1 * 1024 * 1024 // 1 MB cap on each of stdout/stderr capture
)

// Spec describes a command to execute.
type Spec struct {
	Argv    []string
	Timeout time.Duration
	// Env holds the TAI_* contract variables for this command. Every key is
	// always set (possibly to the empty string) so scripts running under
	// `set -u` can reference them without surprise. The user's environment
	// is inherited underneath; see BuildEnv.
	Env map[string]string
}

// Run executes the command and waits for it to finish. Errors distinguish
// timeouts, nonzero exits, and spawn failures, and include a tail of stderr.
func Run(spec Spec) error {
	return execute(spec, BuildEnv(spec.Env), "")
}

// FetchInto runs the command with a tai-managed staging directory exposed via
// TAI_DEST_DIR, verifies the command produced output, and atomically swaps
// the staging directory into dest. On failure the staging directory is
// preserved for debugging and its path is included in the returned error.
//
// stagingParent, when non-empty, is the directory the staging dir is created
// under - pass a directory on the same filesystem as dest to guarantee the
// swap is a true atomic rename. When empty, the system temp dir is used.
func FetchInto(spec Spec, dest, stagingParent string) error {
	staging, err := stage(spec, stagingParent)
	if err != nil {
		return err
	}
	if err := SwapInto(staging, dest); err != nil {
		return fmt.Errorf("swapping staging into %q (staging: %s): %w", dest, staging, err)
	}
	return nil
}

// MergeInto runs the command like FetchInto, but swaps each top-level entry
// the command wrote into dest individually, leaving every other entry of dest
// in place. It is for a dest that already holds content the command must add
// to rather than replace - a skill's root directory, say. Each entry is
// replaced atomically; the merge as a whole is not, so a failure partway can
// leave earlier entries replaced.
//
// A top-level symlink is rejected before anything moves. Merged into dest it
// would point outside it, and whatever writes into dest next - the install
// rewriting a skill's SKILL.md, for one - would write through the link.
//
// It returns the names of the entries it merged.
func MergeInto(spec Spec, dest string) ([]string, error) {
	staging, err := stage(spec, "")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(staging)
	if err != nil {
		return nil, fmt.Errorf("reading staging dir %q: %w", staging, err)
	}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("command %v wrote a symlink, %q, which cannot be merged into %q; write a regular file or directory instead (staging: %s)",
				redactArgv(spec.Argv), e.Name(), dest, staging)
		}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if err := SwapInto(filepath.Join(staging, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return nil, fmt.Errorf("merging %q into %q (staging: %s): %w", e.Name(), dest, staging, err)
		}
		names = append(names, e.Name())
	}
	return names, os.Remove(staging)
}

// stage runs the command with a fresh staging directory exposed via
// TAI_DEST_DIR and returns that directory once the command has written
// something into it.
func stage(spec Spec, stagingParent string) (string, error) {
	var staging string
	var err error
	if stagingParent != "" {
		if err = os.MkdirAll(stagingParent, 0755); err != nil {
			return "", fmt.Errorf("creating staging parent: %w", err)
		}
		staging, err = os.MkdirTemp(stagingParent, "tai-fetch-*")
	} else {
		staging, err = os.MkdirTemp("", "tai-fetch-*")
	}
	if err != nil {
		return "", fmt.Errorf("creating staging dir: %w", err)
	}
	// Staging is consumed by the swap on success; on failure we leave it for
	// debugging and surface the path in the error.

	env := make(map[string]string, len(spec.Env)+1)
	for k, v := range spec.Env {
		env[k] = v
	}
	env["TAI_DEST_DIR"] = staging

	if err := execute(spec, BuildEnv(env), staging); err != nil {
		return "", err
	}

	// Verify the script actually produced output. Silent zero-output success
	// is almost always a bug; surface it loudly with a hint about the
	// contract.
	empty, err := isDirEmpty(staging)
	if err != nil {
		return "", fmt.Errorf("inspecting staging dir %q: %w", staging, err)
	}
	if empty {
		return "", fmt.Errorf("command %v exited 0 but wrote no files; scripts must write into $TAI_DEST_DIR (= %s)",
			redactArgv(spec.Argv), staging)
	}
	return staging, nil
}

// execute runs the command with the given env. staging, when non-empty, is
// mentioned in error messages so users can inspect partial output.
func execute(spec Spec, env []string, staging string) error {
	if len(spec.Argv) == 0 {
		return fmt.Errorf("command is empty (validate should have caught this)")
	}

	timeout := spec.Timeout
	if timeout <= 0 {
		return fmt.Errorf("command timeout must be positive (got %s)", timeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, spec.Argv[0], spec.Argv[1:]...)
	cmd.Env = env
	// Put the child in its own process group (POSIX) so that when the context
	// is cancelled (e.g. on timeout), we can signal the whole group and clean
	// up any grandchildren (e.g. `sh -c "curl ..."` where curl is the long
	// thing). No-op on platforms that don't support process groups.
	applyProcessGroup(cmd)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &cappedWriter{W: &stdoutBuf, Cap: OutputCapBytes}
	cmd.Stderr = &cappedWriter{W: &stderrBuf, Cap: OutputCapBytes}

	stagingNote := ""
	if staging != "" {
		stagingNote = fmt.Sprintf(" (staging: %s)", staging)
	}

	runErr := cmd.Run()
	// A timeout is only a *timeout* if the run actually failed because of it.
	// Otherwise the script may have completed in the same tick the deadline
	// fired, which would produce a misleading "timed out" error for a
	// successful run.
	if runErr != nil && ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command %v timed out after %s; increase 'timeout' if the script needs longer%s\nstderr (tail):\n%s",
			redactArgv(spec.Argv), timeout, stagingNote, tail(stderrBuf.Bytes(), StderrTailBytes))
	}
	if runErr != nil {
		// For ExitError we already extract the exit code into the message;
		// don't also %w it or the formatted output renders the same code twice
		// ("(exit 1): exit status 1").
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return fmt.Errorf("command %v exited %d%s\nstderr (tail):\n%s",
				redactArgv(spec.Argv), exitErr.ExitCode(), stagingNote, tail(stderrBuf.Bytes(), StderrTailBytes))
		}
		// Non-exit failures (spawn errors, etc.) - keep the wrapped cause.
		return fmt.Errorf("command %v failed to run%s: %w\nstderr (tail):\n%s",
			redactArgv(spec.Argv), stagingNote, runErr, tail(stderrBuf.Bytes(), StderrTailBytes))
	}
	return nil
}

// BuildEnv builds the env passed to a command. The user's environment is
// inherited so PATH, secrets in env (e.g. $GITLAB_TOKEN), and HOME are all
// available. Any pre-existing TAI_* variables in the inherited env are
// stripped so tai's authoritative values are unambiguously visible regardless
// of runtime-specific env-deduplication rules. Contract vars are appended in
// sorted key order for determinism.
func BuildEnv(vars map[string]string) []string {
	inherited := os.Environ()
	env := make([]string, 0, len(inherited)+len(vars))
	for _, kv := range inherited {
		if strings.HasPrefix(kv, "TAI_") {
			continue
		}
		env = append(env, kv)
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+vars[k])
	}
	return env
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
