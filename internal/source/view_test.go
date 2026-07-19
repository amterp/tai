package source

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/patch"
	"github.com/amterp/tai/internal/testutil"
)

// viewFixture sets up a source directory and a loader with a cache.
func viewFixture(t *testing.T, exclude ...string) (*testutil.TestEnv, *Loader, *LoadedSource) {
	t.Helper()
	env := testutil.NewTestEnv(t)

	env.SetupAsset("personas", "remote.md", "---\nname: remote\n---\n\nAlways install with npm.\n")

	loader := NewLoader(env.ConfigRepo)
	loader.SetCacheDir(env.CacheDir)

	ls, err := loader.Load(config.Source{
		Name: "upstream", Type: "local", Path: "./assets", Exclude: exclude,
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	return env, loader, ls
}

func patchSet(t *testing.T, dir string, files map[string]string) *patch.Set {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	set, err := patch.LoadDir(dir, "upstream")
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	return set
}

const pnpmPatch = `version: 1
file: personas/remote.md
edits:
  - find: Always install with npm.
    replace: Always install with pnpm.
`

func TestBuildViewAppliesPatchesAndRedirectsContentPath(t *testing.T) {
	env, loader, ls := viewFixture(t)
	set := patchSet(t, filepath.Join(env.Root, "patches"), map[string]string{"pnpm": pnpmPatch})

	if ls.ContentPath() != ls.ResolvedPath {
		t.Fatal("ContentPath should be pristine before a view is built")
	}

	if err := loader.BuildView(ls, set); err != nil {
		t.Fatalf("BuildView failed: %v", err)
	}

	if ls.ViewPath == "" || ls.ContentPath() != ls.ViewPath {
		t.Fatalf("ContentPath = %q, want the view at %q", ls.ContentPath(), ls.ViewPath)
	}
	if got := env.ReadFile(filepath.Join(ls.ViewPath, "personas", "remote.md")); !strings.Contains(got, "pnpm") {
		t.Errorf("view content = %q, want it patched", got)
	}
	// Pristine must survive untouched - drift detection depends on it.
	if got := env.ReadFile(filepath.Join(ls.ResolvedPath, "personas", "remote.md")); strings.Contains(got, "pnpm") {
		t.Error("BuildView wrote to the pristine source")
	}
}

func TestBuildViewWithNoPatchesIsNoop(t *testing.T) {
	_, loader, ls := viewFixture(t)

	if err := loader.BuildView(ls, patch.NewSet("upstream", nil)); err != nil {
		t.Fatalf("BuildView failed: %v", err)
	}
	if ls.ViewPath != "" {
		t.Error("a source with no patches should not get a view")
	}
}

func TestBuildViewSkipsGitDirectory(t *testing.T) {
	env, loader, ls := viewFixture(t)

	// A local path source is usually a git checkout; copying .git would dwarf
	// the content it exists to carry.
	gitDir := filepath.Join(ls.ResolvedPath, ".git", "objects")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "blob"), []byte("junk"), 0644); err != nil {
		t.Fatal(err)
	}

	set := patchSet(t, filepath.Join(env.Root, "patches"), map[string]string{"pnpm": pnpmPatch})
	if err := loader.BuildView(ls, set); err != nil {
		t.Fatalf("BuildView failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(ls.ViewPath, ".git")); !os.IsNotExist(err) {
		t.Error(".git was copied into the view")
	}
}

func TestBuildViewExcludesDroppedPaths(t *testing.T) {
	env, loader, ls := viewFixture(t, "personas/secret.md")
	if err := os.WriteFile(filepath.Join(ls.ResolvedPath, "personas", "secret.md"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	set := patchSet(t, filepath.Join(env.Root, "patches"), map[string]string{"pnpm": pnpmPatch})
	if err := loader.BuildView(ls, set); err != nil {
		t.Fatalf("BuildView failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(ls.ViewPath, "personas", "secret.md")); !os.IsNotExist(err) {
		t.Error("excluded file was copied into the view")
	}
}

func TestBuildViewFailureKeepsWorkspaceAndPreviousView(t *testing.T) {
	env, loader, ls := viewFixture(t)
	patchDir := filepath.Join(env.Root, "patches")

	// Build a good view first, so there is a previous view to protect.
	if err := loader.BuildView(ls, patchSet(t, patchDir, map[string]string{"pnpm": pnpmPatch})); err != nil {
		t.Fatalf("first BuildView failed: %v", err)
	}
	goodView := ls.ViewPath

	// Now a patch that cannot apply.
	broken := patchSet(t, filepath.Join(env.Root, "broken"), map[string]string{"nope": `version: 1
file: personas/remote.md
edits:
  - find: text that is not there
    replace: x
`})

	err := loader.BuildView(ls, broken)
	var viewErr *ViewError
	if !errors.As(err, &viewErr) {
		t.Fatalf("err = %v (%T), want *ViewError", err, err)
	}
	if viewErr.ApplyErr == nil || viewErr.Patch.Name != "nope" {
		t.Errorf("ViewError = %+v, want it to name the failing patch", viewErr)
	}

	// Repairing a patch means inspecting what it failed against.
	if viewErr.Workspace == "" {
		t.Fatal("no workspace reported")
	}
	if _, statErr := os.Stat(filepath.Join(viewErr.Workspace, "personas", "remote.md")); statErr != nil {
		t.Errorf("workspace not usable: %v", statErr)
	}

	// A failed rebuild must not destroy what was already installed from.
	if got := env.ReadFile(filepath.Join(goodView, "personas", "remote.md")); !strings.Contains(got, "pnpm") {
		t.Error("a failed build damaged the previous view")
	}
}

func TestBuildViewRebuildsFromPristineEachTime(t *testing.T) {
	env, loader, ls := viewFixture(t)
	set := patchSet(t, filepath.Join(env.Root, "patches"), map[string]string{"pnpm": pnpmPatch})

	if err := loader.BuildView(ls, set); err != nil {
		t.Fatalf("BuildView failed: %v", err)
	}
	// Rebuilding must re-apply against pristine, not against the previous view -
	// applying to already-patched content would fail exactly-once matching.
	if err := loader.BuildView(ls, set); err != nil {
		t.Fatalf("second BuildView failed: %v", err)
	}
	if got := env.ReadFile(filepath.Join(ls.ViewPath, "personas", "remote.md")); !strings.Contains(got, "pnpm") {
		t.Errorf("view content = %q, want it patched after rebuild", got)
	}
}
