package source

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/amterp/tai/internal/cmdexec"
	"github.com/amterp/tai/internal/patch"
	"github.com/amterp/tai/internal/pathutil"
)

// ViewError reports a patch that could not be applied while materializing a
// source view. It carries the staging directory, which is deliberately left on
// disk: repairing a patch means looking at the content it failed against, and
// an error that deletes the evidence pushes people to work around the feature
// instead of fixing the patch.
type ViewError struct {
	SourceName string
	Patch      *patch.Patch
	ApplyErr   *patch.ApplyError // nil when the failure was not an apply failure
	Reason     string            // set when ApplyErr is nil
	Workspace  string
}

func (e *ViewError) Error() string {
	detail := e.Reason
	if e.ApplyErr != nil {
		detail = e.ApplyErr.Error()
	}
	return fmt.Sprintf("source %q: %s", e.SourceName, detail)
}

// BuildView materializes a patched copy of a source and points the source at
// it, so everything downstream reads patched content without knowing patches
// exist. The pristine source is never written to: it stays an honest mirror of
// upstream, which is what lets blessed snapshots detect real upstream drift
// rather than tai's own edits.
//
// Rebuilds unconditionally. A cache keyed on source and patch content would
// save tens of milliseconds and introduce a way to silently install stale
// content, which is the exact failure patches exist to prevent.
func (l *Loader) BuildView(ls *LoadedSource, patches *patch.Set) error {
	if patches.Len() == 0 {
		return nil
	}
	if err := ls.RequireMaterialized(); err != nil {
		return err
	}

	viewPath, err := l.viewCachePath(ls.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(viewPath), 0755); err != nil {
		return fmt.Errorf("creating view directory: %w", err)
	}

	staging, err := l.newStagingDir("view-" + ls.Name + "-")
	if err != nil {
		return err
	}

	if err := l.copyForView(ls, staging); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}

	if err := applyPatchesInDir(staging, ls.Name, patches); err != nil {
		var viewErr *ViewError
		if errors.As(err, &viewErr) {
			// Promote the half-built tree to the source's stable workspace, so
			// the failure message and `tai patch regenerate` name one place
			// rather than a temp path the user has to copy around.
			if wsPath, wsErr := l.workspacePath(ls.Name); wsErr == nil {
				if swapErr := cmdexec.SwapInto(staging, wsPath); swapErr == nil {
					viewErr.Workspace = wsPath
					return err
				}
			}
			viewErr.Workspace = staging
		}
		return err
	}

	if err := cmdexec.SwapInto(staging, viewPath); err != nil {
		return fmt.Errorf("installing view for source %q: %w", ls.Name, err)
	}

	ls.ViewPath = viewPath
	return nil
}

// copyForView copies the pristine source into staging, dropping excluded paths
// and version-control metadata.
func (l *Loader) copyForView(ls *LoadedSource, staging string) error {
	return filepath.WalkDir(ls.ResolvedPath, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(ls.ResolvedPath, srcPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(staging, 0755)
		}

		// A local path source is usually a git checkout, where .git dwarfs the
		// content and copying it would turn a ~20ms build into seconds. Implicit
		// and separate from the user's `exclude:` patterns.
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		if ls.IsExcluded(filepath.ToSlash(rel)) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		dstPath := filepath.Join(staging, rel)

		// Symlinks are followed, not skipped. Asset discovery follows them, so
		// a view that dropped them would advertise assets it cannot produce -
		// declaring `patches:` for one file would silently stop installing every
		// symlinked asset in the source. Copying the target keeps the view a
		// faithful stand-in for what discovery saw.
		if d.Type()&fs.ModeSymlink != 0 {
			info, statErr := os.Stat(srcPath)
			if statErr != nil {
				// A dangling link is not ours to resolve; discovery ignores it
				// too, so leaving it out keeps the two consistent.
				return nil
			}
			if info.IsDir() {
				// Resolved first: WalkDir does not descend when its root is
				// itself a symlink, so copying via the link name silently
				// produces nothing.
				target, resolveErr := filepath.EvalSymlinks(srcPath)
				if resolveErr != nil {
					return nil
				}
				return pathutil.CopyDir(target, dstPath)
			}
			return pathutil.CopyFile(srcPath, dstPath)
		}

		if d.IsDir() {
			return pathutil.MkdirPreservingMode(dstPath, d)
		}
		return pathutil.CopyFile(srcPath, dstPath)
	})
}

// applyPatchesInDir rewrites the copied files in place. Each patched file is
// read once, has every patch targeting it applied in set order, and is written
// back with its original permissions.
func applyPatchesInDir(dir, sourceName string, patches *patch.Set) error {
	for _, rel := range patches.Files() {
		filePath := filepath.Join(dir, filepath.FromSlash(rel))
		forFile := patches.ForFile(rel)

		content, err := os.ReadFile(filePath)
		if err != nil {
			// Covers both "deleted upstream" and "excluded by this very
			// source", which read the same way to the user: the patch no
			// longer names something this source carries.
			return &ViewError{
				SourceName: sourceName,
				Patch:      forFile[0],
				Reason: fmt.Sprintf("patch %q targets %s, which this source does not carry - it may have been renamed or removed upstream, or excluded by this source's 'exclude' patterns",
					forFile[0].Name, forFile[0].FileRef()),
			}
		}

		patched, _, applyErr := patch.ApplyAll(content, forFile)
		if applyErr != nil {
			return &ViewError{SourceName: sourceName, Patch: applyErr.Patch, ApplyErr: applyErr}
		}

		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(filePath); statErr == nil {
			mode = info.Mode().Perm()
		}
		if err := os.WriteFile(filePath, patched, mode); err != nil {
			return fmt.Errorf("writing patched %s: %w", rel, err)
		}
	}
	return nil
}

// BuildWorkspace materializes a source into a stable, editable directory with
// every patch that applies already applied, and returns its path along with the
// names of any patches that could not be applied.
//
// This is where a patch is authored or repaired: you edit the file normally and
// `tai patch regenerate` works out the edits. A stable path per source means
// neither command needs the location passed around.
func (l *Loader) BuildWorkspace(ls *LoadedSource, patches *patch.Set) (string, []string, error) {
	if err := ls.RequireMaterialized(); err != nil {
		return "", nil, err
	}

	wsPath, err := l.workspacePath(ls.Name)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(wsPath), 0755); err != nil {
		return "", nil, fmt.Errorf("creating workspace directory: %w", err)
	}

	staging, err := l.newStagingDir("workspace-" + ls.Name + "-")
	if err != nil {
		return "", nil, err
	}
	if err := l.copyForView(ls, staging); err != nil {
		_ = os.RemoveAll(staging)
		return "", nil, err
	}

	// Unlike a view build, a patch that cannot apply is expected here - it is
	// the reason the user opened a workspace. Apply what works and name the
	// rest so they know what to re-make by hand.
	var skipped []string
	for _, rel := range patches.Files() {
		filePath := filepath.Join(staging, filepath.FromSlash(rel))
		content, readErr := os.ReadFile(filePath)
		if readErr != nil {
			for _, p := range patches.ForFile(rel) {
				skipped = append(skipped, p.Name)
			}
			continue
		}
		for _, p := range patches.ForFile(rel) {
			next, applyErr := patch.Apply(content, p)
			if applyErr != nil {
				skipped = append(skipped, p.Name)
				continue
			}
			content = next
		}
		if err := os.WriteFile(filePath, content, 0o644); err != nil {
			_ = os.RemoveAll(staging)
			return "", nil, fmt.Errorf("writing %s: %w", rel, err)
		}
	}

	if err := cmdexec.SwapInto(staging, wsPath); err != nil {
		return "", nil, fmt.Errorf("creating workspace for source %q: %w", ls.Name, err)
	}
	return wsPath, skipped, nil
}

// RegenerationBaseline is the content a patch's edits are measured against:
// pristine, with every *other* patch on the same file applied.
//
// Excluding only the patch being regenerated is what stops regeneration from
// absorbing a sibling's changes - the failure mode that makes a whole-directory
// `git diff` unusable once a source has more than one patch.
func (ls *LoadedSource) RegenerationBaseline(patches *patch.Set, target *patch.Patch) ([]byte, error) {
	content, err := ls.PristineContent(target.File)
	if err != nil {
		return nil, err
	}
	for _, p := range patches.ForFile(target.File) {
		if p.Name == target.Name {
			continue
		}
		next, applyErr := patch.Apply(content, p)
		if applyErr != nil {
			return nil, fmt.Errorf("cannot establish a baseline: sibling patch %q on the same file does not apply (%s)", p.Name, applyErr.Error())
		}
		content = next
	}
	return content, nil
}

// WorkspacePath returns where this source's editable workspace lives.
func (l *Loader) WorkspacePath(sourceName string) (string, error) {
	return l.workspacePath(sourceName)
}

func (l *Loader) workspacePath(sourceName string) (string, error) {
	if l.cacheDir == "" {
		return "", fmt.Errorf("source %q: no cache directory configured", sourceName)
	}
	return filepath.Join(l.cacheDir, "workspaces", l.repoKey(), sourceName), nil
}

func (l *Loader) newStagingDir(prefix string) (string, error) {
	stagingParent := filepath.Join(l.cacheDir, "staging")
	if err := os.MkdirAll(stagingParent, 0755); err != nil {
		return "", fmt.Errorf("creating staging directory: %w", err)
	}
	dir, err := os.MkdirTemp(stagingParent, prefix)
	if err != nil {
		return "", fmt.Errorf("creating staging directory: %w", err)
	}
	return dir, nil
}

// InspectPatches evaluates every patch in the set against this source's
// pristine content, in set order. Used by the patch commands and by install to
// warn about drift before building the view.
func (ls *LoadedSource) InspectPatches(patches *patch.Set) ([]*patch.Inspection, error) {
	pristine, baselines := ls.inspectionBaselines(patches)

	out := make([]*patch.Inspection, 0, patches.Len())
	for _, p := range patches.All() {
		if !ls.Materialized {
			out = append(out, patch.MissingInspection(p, patch.StatusSourceUnfetched))
			continue
		}
		content, ok := pristine[p.File]
		if !ok {
			out = append(out, patch.MissingInspection(p, patch.StatusFileMissing))
			continue
		}
		insp, err := patch.Inspect(content, baselines[p.Name], p)
		if err != nil {
			return nil, err
		}
		out = append(out, insp)
	}
	return out, nil
}

// inspectionBaselines returns each patched file's pristine content, and the
// content each patch is applied against once its earlier same-file siblings
// have run.
//
// This mirrors applyPatchesInDir deliberately: inspection that disagreed with
// the install pipeline about whether a patch applies would report healthy
// patches as broken, and blessing refuses a broken patch - so the drift warning
// could never be cleared.
func (ls *LoadedSource) inspectionBaselines(patches *patch.Set) (map[string][]byte, map[string][]byte) {
	pristine := make(map[string][]byte)
	baselines := make(map[string][]byte)
	if !ls.Materialized {
		return pristine, baselines
	}

	for _, rel := range patches.Files() {
		content, err := ls.PristineContent(rel)
		if err != nil {
			continue // reported per-patch as StatusFileMissing
		}
		pristine[rel] = content

		current := content
		for _, p := range patches.ForFile(rel) {
			baselines[p.Name] = current
			next, applyErr := patch.Apply(current, p)
			if applyErr != nil {
				// Leave the baseline where it is. This patch is about to be
				// reported broken, which is the actionable finding; guessing at
				// content it never produced would only mislead the ones after it.
				continue
			}
			current = next
		}
	}
	return pristine, baselines
}

// PristineContent reads a source-relative file from the unpatched source.
// Patch inspection and blessing must both work from upstream, never from a
// view tai produced.
func (ls *LoadedSource) PristineContent(rel string) ([]byte, error) {
	if err := ls.RequireMaterialized(); err != nil {
		return nil, err
	}
	full := filepath.Join(ls.PristinePath(), filepath.FromSlash(rel))
	if !pathutil.IsContainedIn(full, ls.PristinePath()) {
		return nil, fmt.Errorf("path %q escapes source %q", rel, ls.Name)
	}
	return os.ReadFile(full)
}

// viewCachePath is where a source's patched view is materialized. It sits
// beside the fetched content of command sources rather than inside it, so a
// command source with patches keeps its pristine fetch and its view separate.
func (l *Loader) viewCachePath(sourceName string) (string, error) {
	if l.cacheDir == "" {
		return "", fmt.Errorf("source %q: no cache directory configured for patched sources", sourceName)
	}
	return filepath.Join(l.cacheDir, "views", l.repoKey(), sourceName), nil
}
