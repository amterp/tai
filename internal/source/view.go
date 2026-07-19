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

	stagingParent := filepath.Join(l.cacheDir, "staging")
	if err := os.MkdirAll(stagingParent, 0755); err != nil {
		return fmt.Errorf("creating staging directory: %w", err)
	}
	staging, err := os.MkdirTemp(stagingParent, "view-"+ls.Name+"-")
	if err != nil {
		return fmt.Errorf("creating staging directory: %w", err)
	}

	if err := l.copyForView(ls, staging); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}

	if err := applyPatchesInDir(staging, ls.Name, patches); err != nil {
		// Staging is intentionally not removed - see ViewError.
		var viewErr *ViewError
		if errors.As(err, &viewErr) {
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
		// Symlinks are skipped for the same reason CopyDir skips them: a link
		// out of the source is an escape hatch we don't want to materialize.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
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

// viewCachePath is where a source's patched view is materialized. It sits
// beside the fetched content of command sources rather than inside it, so a
// command source with patches keeps its pristine fetch and its view separate.
func (l *Loader) viewCachePath(sourceName string) (string, error) {
	if l.cacheDir == "" {
		return "", fmt.Errorf("source %q: no cache directory configured for patched sources", sourceName)
	}
	return filepath.Join(l.cacheDir, "views", l.repoKey(), sourceName), nil
}
