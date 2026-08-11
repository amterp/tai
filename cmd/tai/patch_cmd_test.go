package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/output"
	"github.com/amterp/tai/internal/patch"
	"github.com/amterp/tai/internal/state"
	"github.com/amterp/tai/internal/testutil"
)

// These helpers back `tai patch list/diff/bless/workspace/regenerate`. They
// were untested, which mattered once url sources joined the set of sources
// inspectAllPatches has to walk.
//
// Only the resolving paths are covered: the failure paths call fatal(), which
// exits the process.

// newPatchCLIEnv builds a config repo whose sources all declare patches, and
// returns a State wired the way loadState() would wire it.
func newPatchCLIEnv(t *testing.T, extraSources string) (*testutil.TestEnv, *state.State) {
	t.Helper()

	// fatal() writes through the package-level `out`; give it somewhere to go
	// so an unexpected code path fails loudly instead of nil-panicking.
	out = output.NewPlain(io.Discard, io.Discard)

	env := testutil.NewTestEnv(t)

	upstream := filepath.Join(env.Root, "upstream")
	if err := os.MkdirAll(filepath.Join(upstream, "personas"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(upstream, "personas", "remote.md"),
		[]byte("---\nname: remote\ndescription: Remote\n---\n\nAlways install with npm.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: upstream
    path: ` + upstream + `
    patches: patches/upstream/
` + extraSources)

	env.WriteConfigRepoFile(filepath.Join("patches", "upstream", "use-pnpm.yaml"), `version: 1
file: personas/remote.md
description: prefer pnpm
edits:
  - find: Always install with npm.
    replace: Always install with pnpm.
`)

	paths := &state.Paths{
		ConfigDir:   env.ConfigDir,
		CacheDir:    env.CacheDir,
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}
	s := &state.State{
		Paths:        paths,
		GlobalConfig: &state.GlobalConfig{ActiveConfig: env.ConfigRepo},
	}
	if err := s.LoadConfigRepo(env.ConfigRepo); err != nil {
		t.Fatalf("LoadConfigRepo() error = %v", err)
	}
	return env, s
}

func TestInspectAllPatches(t *testing.T) {
	t.Run("reports an unblessed patch that applies", func(t *testing.T) {
		_, s := newPatchCLIEnv(t, "")

		refs := inspectAllPatches(s)
		if len(refs) != 1 {
			t.Fatalf("got %d patches, want 1", len(refs))
		}
		r := refs[0]
		if got := r.qualifiedName(); got != "upstream:use-pnpm" {
			t.Errorf("qualifiedName() = %q, want %q", got, "upstream:use-pnpm")
		}
		if r.insp.Status != patch.StatusUnblessed {
			t.Errorf("Status = %v, want StatusUnblessed", r.insp.Status)
		}
		if reason := blessBlocker(r.insp); reason != "" {
			t.Errorf("blessBlocker() = %q, want it blessable", reason)
		}
	})

	t.Run("reports a broken patch without failing the walk", func(t *testing.T) {
		env, s := newPatchCLIEnv(t, "")
		env.WriteConfigRepoFile(filepath.Join("patches", "upstream", "use-pnpm.yaml"), `version: 1
file: personas/remote.md
edits:
  - find: This text is not in upstream.
    replace: Nor is this.
`)
		if err := s.LoadConfigRepo(s.GlobalConfig.ActiveConfig); err != nil {
			t.Fatal(err)
		}

		refs := inspectAllPatches(s)
		if len(refs) != 1 {
			t.Fatalf("got %d patches, want 1", len(refs))
		}
		if refs[0].insp.Status != patch.StatusBroken {
			t.Fatalf("Status = %v, want StatusBroken", refs[0].insp.Status)
		}
		reason := blessBlocker(refs[0].insp)
		if !strings.Contains(reason, "does not apply") {
			t.Errorf("blessBlocker() = %q, want it to refuse a broken patch", reason)
		}
	})

	t.Run("walks a url source that has not been fetched", func(t *testing.T) {
		env, s := newPatchCLIEnv(t, `  - name: upstream-guide
    type: url
    url: https://example.com/GUIDE.md
    patches: patches/upstream-guide/
`)
		env.WriteConfigRepoFile(filepath.Join("patches", "upstream-guide", "use-pnpm.yaml"), `version: 1
file: GUIDE.md
edits:
  - find: npm
    replace: pnpm
`)
		if err := s.LoadConfigRepo(s.GlobalConfig.ActiveConfig); err != nil {
			t.Fatal(err)
		}

		refs := inspectAllPatches(s)
		if len(refs) != 2 {
			t.Fatalf("got %d patches, want 2 (one per source)", len(refs))
		}

		var guide *patchRef
		for i := range refs {
			if refs[i].source.Name == "upstream-guide" {
				guide = &refs[i]
			}
		}
		if guide == nil {
			t.Fatal("the url source's patch was not inspected")
		}
		if guide.insp.Status != patch.StatusSourceUnfetched {
			t.Errorf("Status = %v, want StatusSourceUnfetched before the first fetch", guide.insp.Status)
		}
		if reason := blessBlocker(guide.insp); !strings.Contains(reason, "tai refresh") {
			t.Errorf("blessBlocker() = %q, want it to point at 'tai refresh'", reason)
		}
	})

	t.Run("inspects a url source after it is fetched", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Always install with npm.\n"))
		}))
		defer server.Close()

		env, s := newPatchCLIEnv(t, `  - name: upstream-guide
    type: url
    url: `+server.URL+`/GUIDE.md
    patches: patches/upstream-guide/
`)
		env.WriteConfigRepoFile(filepath.Join("patches", "upstream-guide", "use-pnpm.yaml"), `version: 1
file: GUIDE.md
edits:
  - find: Always install with npm.
    replace: Always install with pnpm.
`)
		if err := s.RefreshSources("upstream-guide", nil); err != nil {
			t.Fatalf("RefreshSources() error = %v", err)
		}

		refs := inspectAllPatches(s)
		var guide *patchRef
		for i := range refs {
			if refs[i].source.Name == "upstream-guide" {
				guide = &refs[i]
			}
		}
		if guide == nil {
			t.Fatal("the url source's patch was not inspected")
		}
		if guide.insp.Status != patch.StatusUnblessed {
			t.Errorf("Status = %v, want StatusUnblessed (applies, no snapshot yet)", guide.insp.Status)
		}
		if reason := blessBlocker(guide.insp); reason != "" {
			t.Errorf("blessBlocker() = %q, want a fetched url source's patch to be blessable", reason)
		}
	})
}

func TestFindPatch(t *testing.T) {
	env, s := newPatchCLIEnv(t, "")
	env.WriteConfigRepoFile(filepath.Join("patches", "upstream", "second.yaml"), `version: 1
file: personas/remote.md
edits:
  - find: "description: Remote"
    replace: "description: Remote persona"
`)
	if err := s.LoadConfigRepo(s.GlobalConfig.ActiveConfig); err != nil {
		t.Fatal(err)
	}

	t.Run("bare name resolves when unambiguous", func(t *testing.T) {
		if got := findPatch(s, "second").qualifiedName(); got != "upstream:second" {
			t.Errorf("findPatch(%q) = %q, want %q", "second", got, "upstream:second")
		}
	})

	t.Run("qualified name resolves", func(t *testing.T) {
		if got := findPatch(s, "upstream:use-pnpm").qualifiedName(); got != "upstream:use-pnpm" {
			t.Errorf("findPatch() = %q, want %q", got, "upstream:use-pnpm")
		}
	})
}

func TestBlessBlocker(t *testing.T) {
	p := &patch.Patch{Name: "p", File: "a.md", SourceName: "upstream"}

	tests := []struct {
		name   string
		insp   *patch.Inspection
		reject string // substring the reason must contain; "" means blessable
	}{
		{name: "ok", insp: &patch.Inspection{Patch: p, Status: patch.StatusOK}},
		{name: "drifted", insp: &patch.Inspection{Patch: p, Status: patch.StatusDrifted}},
		{name: "unblessed", insp: &patch.Inspection{Patch: p, Status: patch.StatusUnblessed}},
		{
			name:   "broken",
			insp:   &patch.Inspection{Patch: p, Status: patch.StatusBroken, ApplyErr: &patch.ApplyError{Patch: p, Find: "x"}},
			reject: "does not apply",
		},
		{
			name:   "unfetched",
			insp:   &patch.Inspection{Patch: p, Status: patch.StatusSourceUnfetched},
			reject: "tai refresh",
		},
		{
			name:   "file missing",
			insp:   &patch.Inspection{Patch: p, Status: patch.StatusFileMissing},
			reject: "does not carry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blessBlocker(tt.insp)
			if tt.reject == "" {
				if got != "" {
					t.Errorf("blessBlocker() = %q, want it blessable", got)
				}
				return
			}
			if !strings.Contains(got, tt.reject) {
				t.Errorf("blessBlocker() = %q, want it to contain %q", got, tt.reject)
			}
		})
	}
}

// Guards the assumption inspectAllPatches relies on: a source declaring no
// patches contributes nothing rather than erroring.
func TestInspectAllPatchesIgnoresSourcesWithoutPatches(t *testing.T) {
	out = output.NewPlain(io.Discard, io.Discard)

	env := testutil.NewTestEnv(t)
	env.SetupTaiConfig("version: 1\nsources:\n  - name: local\n    path: ./assets\n")
	env.SetupAsset("personas", "p.md", "---\nname: p\ndescription: P\n---\n\nBody\n")

	s := &state.State{
		Paths: &state.Paths{
			ConfigDir:   env.ConfigDir,
			CacheDir:    env.CacheDir,
			ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
			ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
		},
		GlobalConfig: &state.GlobalConfig{ActiveConfig: env.ConfigRepo},
	}
	if err := s.LoadConfigRepo(env.ConfigRepo); err != nil {
		t.Fatal(err)
	}

	if refs := inspectAllPatches(s); len(refs) != 0 {
		t.Errorf("got %d patches, want 0", len(refs))
	}
}
