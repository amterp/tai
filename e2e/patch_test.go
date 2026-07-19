package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/patch"
	"github.com/amterp/tai/internal/profile"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/testutil"
)

// patchEnv is a config repo importing an "upstream" source it does not own,
// with a patch directory declared on that source.
type patchEnv struct {
	*testutil.TestEnv
	upstream string
	loader   *source.Loader
	taiCfg   *config.TaiConfig
}

// newPatchEnv builds the config repo. exclude may be empty.
func newPatchEnv(t *testing.T, exclude ...string) *patchEnv {
	t.Helper()
	env := testutil.NewTestEnv(t)

	upstream := filepath.Join(env.Root, "upstream")
	for _, d := range []string{"personas", "delegates", "skills", "statuslines"} {
		if err := os.MkdirAll(filepath.Join(upstream, d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	cfg := `version: 1
sources:
  - name: local
    path: ./assets
  - name: upstream
    path: ` + upstream + `
    patches: patches/upstream/
`
	for _, e := range exclude {
		if !strings.Contains(cfg, "exclude:") {
			cfg += "    exclude:\n"
		}
		cfg += "      - \"" + e + "\"\n"
	}
	env.SetupTaiConfig(cfg)
	env.SetupProfile("test", "name: test\ntarget: claude\nbundles:\n  - test\n")

	taiCfg, err := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	loader := source.NewLoader(env.ConfigRepo)
	loader.SetCacheDir(env.CacheDir)

	return &patchEnv{TestEnv: env, upstream: upstream, loader: loader, taiCfg: taiCfg}
}

func (e *patchEnv) writeUpstream(rel, content string) {
	e.T.Helper()
	path := filepath.Join(e.upstream, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		e.T.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		e.T.Fatal(err)
	}
}

func (e *patchEnv) writePatch(name, content string) {
	e.T.Helper()
	e.WriteConfigRepoFile(filepath.Join("patches", "upstream", name+".yaml"), content)
}

// installOnce runs the real pipeline: load sources, build patched views,
// resolve the profile, install. Mirrors what the CLI does per invocation.
func (e *patchEnv) installOnce(t *testing.T) (*install.InstallResult, error) {
	t.Helper()

	sources, err := e.loader.LoadAll(e.taiCfg.Sources)
	if err != nil {
		t.Fatal(err)
	}

	for _, src := range sources {
		if !src.HasPatches() {
			continue
		}
		patches, err := patch.LoadDir(filepath.Join(e.ConfigRepo, filepath.FromSlash(src.Patches)), src.Name)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.loader.BuildView(src, patches); err != nil {
			return nil, err
		}
	}

	bundles, err := config.LoadBundlesFromDir(filepath.Join(e.ConfigRepo, "bundles"))
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := config.LoadProfilesFromDir(filepath.Join(e.ConfigRepo, "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := profile.NewResolver(profiles, bundles, sources).Resolve("test")
	if err != nil {
		return nil, err
	}

	installer := install.NewInstaller(
		install.NewManifestStore(filepath.Join(e.ConfigDir, "manifests")), e.ConfigRepo, sources)
	return installer.Install(claude.New(e.TargetDir), "test", resolved.Assets, install.InstallOptions{})
}

func TestE2EPatchedSourceInstallsPatchedAndLeavesUpstreamAlone(t *testing.T) {
	env := newPatchEnv(t)
	env.writeUpstream("personas/remote.md", "---\nname: remote\ndescription: Remote\n---\n\nAlways install with npm.\n")
	env.SetupBundle("test", "name: test\nassets:\n  - type: persona\n    path: upstream:personas/remote.md\n")
	env.writePatch("use-pnpm", `version: 1
file: personas/remote.md
edits:
  - find: Always install with npm.
    replace: Always install with pnpm.
`)

	if _, err := env.installOnce(t); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if got := env.ReadTargetFile("output-styles/remote.md"); !strings.Contains(got, "pnpm") {
		t.Errorf("installed content = %q, want it patched", got)
	}

	// The pristine source must never be written to - blessed-snapshot drift
	// detection is only meaningful if upstream stays an honest mirror.
	upstream := env.ReadFile(filepath.Join(env.upstream, "personas", "remote.md"))
	if !strings.Contains(upstream, "npm.") || strings.Contains(upstream, "pnpm") {
		t.Errorf("upstream = %q, want it untouched", upstream)
	}
}

// Regression: dependency resolution reads asset content directly. Under
// read-time patching it read pristine bytes, so a patch touching `depends:`
// was silently ignored and tai installed a different dependency set than the
// patched content declared.
func TestE2EPatchedFrontmatterDependencyIsHonored(t *testing.T) {
	env := newPatchEnv(t)
	env.writeUpstream("personas/remote.md", "---\nname: remote\ndescription: Remote\n---\n\nBody\n")
	env.writeUpstream("delegates/helper.md", "---\nname: helper\ndescription: Helper\n---\n\nHelper body\n")
	env.SetupBundle("test", "name: test\nassets:\n  - type: persona\n    path: upstream:personas/remote.md\n")

	// The bundle names only the persona; the patch adds the dependency.
	env.writePatch("add-dep", `version: 1
file: personas/remote.md
edits:
  - find: |-
      description: Remote
    replace: |-
      description: Remote
      depends:
        - upstream:delegates/helper.md
`)

	if _, err := env.installOnce(t); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	if !env.TargetFileExists("agents/helper.md") {
		t.Error("patched 'depends' was ignored: the dependency was not installed")
	}
}

// Regression: statusline installation parses the asset from its source path.
// Under read-time patching that read pristine bytes while preflight reported
// the patch healthy - silent wrong content behind a green check.
func TestE2EPatchedStatuslineInstallsPatched(t *testing.T) {
	env := newPatchEnv(t)
	env.writeUpstream("statuslines/bar.yaml", "level: minimal\n")
	env.SetupBundle("test", "name: test\nassets:\n  - type: statusline\n    path: upstream:statuslines/bar.yaml\n")
	env.writePatch("detailed", `version: 1
file: statuslines/bar.yaml
edits:
  - find: "level: minimal"
    replace: "level: detailed"
`)

	if _, err := env.installOnce(t); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	settings := env.ReadTargetFile("settings.json")
	if strings.Contains(settings, "minimal") {
		t.Errorf("settings.json = %q, want the patched statusline, not the pristine one", settings)
	}
}

func TestE2EPatchedSkillDirAndIncludeSeePatchedContent(t *testing.T) {
	env := newPatchEnv(t)
	env.writeUpstream("skills/rad/SKILL.md", "---\nname: rad\ndescription: Rad\n---\n\nAlways install with npm.\n")
	env.writeUpstream("skills/rad/refs/guide.md", "Run npm test.\n")
	env.SetupBundle("test", "name: test\nassets:\n  - type: skill\n    path: upstream:skills/rad\n")
	env.writePatch("rad-main", `version: 1
file: skills/rad/SKILL.md
edits:
  - find: Always install with npm.
    replace: Always install with pnpm.
`)
	env.writePatch("rad-guide", `version: 1
file: skills/rad/refs/guide.md
edits:
  - find: Run npm test.
    replace: Run pnpm test.
`)

	result, err := env.installOnce(t)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if len(result.Added) != 1 {
		t.Fatalf("Added = %v, want 1 asset", result.Added)
	}

	if got := env.ReadTargetFile("skills/rad/SKILL.md"); !strings.Contains(got, "pnpm") {
		t.Errorf("SKILL.md = %q, want it patched", got)
	}
	if got := env.ReadTargetFile("skills/rad/refs/guide.md"); got != "Run pnpm test.\n" {
		t.Errorf("nested guide.md = %q, want it patched", got)
	}

	// Reinstalling must be a no-op: the installed checksum has to cover patched
	// bytes, or every install would report a phantom update.
	result2, err := env.installOnce(t)
	if err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	if len(result2.Unchanged) != 1 {
		t.Errorf("Unchanged = %v, want the skill unchanged on reinstall", result2.Unchanged)
	}
}

func TestE2EPatchFailureAbortsInstallAndKeepsWorkspace(t *testing.T) {
	env := newPatchEnv(t)
	env.writeUpstream("personas/remote.md", "---\nname: remote\ndescription: Remote\n---\n\nUpstream moved on.\n")
	env.SetupBundle("test", "name: test\nassets:\n  - type: persona\n    path: upstream:personas/remote.md\n")
	env.writePatch("use-pnpm", `version: 1
file: personas/remote.md
edits:
  - find: Always install with npm.
    replace: Always install with pnpm.
`)

	_, err := env.installOnce(t)
	var viewErr *source.ViewError
	if !errors.As(err, &viewErr) {
		t.Fatalf("err = %v (%T), want *source.ViewError", err, err)
	}
	if viewErr.Patch.Name != "use-pnpm" {
		t.Errorf("failure names %q, want use-pnpm", viewErr.Patch.Name)
	}
	if viewErr.ApplyErr == nil || viewErr.ApplyErr.Matches != 0 {
		t.Errorf("ApplyErr = %+v, want a not-found failure", viewErr.ApplyErr)
	}
	if env.TargetFileExists("output-styles/remote.md") {
		t.Error("a patch failure must leave the target untouched")
	}

	// Repairing a patch means looking at the content it failed against, so the
	// workspace has to survive the failure.
	if viewErr.Workspace == "" {
		t.Fatal("no workspace reported")
	}
	if _, statErr := os.Stat(filepath.Join(viewErr.Workspace, "personas", "remote.md")); statErr != nil {
		t.Errorf("workspace not usable: %v", statErr)
	}
}

func TestE2EPatchTargetingMissingFileFails(t *testing.T) {
	env := newPatchEnv(t)
	env.writeUpstream("personas/remote.md", "---\nname: remote\ndescription: Remote\n---\n\nBody\n")
	env.SetupBundle("test", "name: test\nassets:\n  - type: persona\n    path: upstream:personas/remote.md\n")
	env.writePatch("gone", `version: 1
file: personas/renamed-away.md
edits:
  - find: a
    replace: b
`)

	_, err := env.installOnce(t)
	var viewErr *source.ViewError
	if !errors.As(err, &viewErr) {
		t.Fatalf("err = %v (%T), want *source.ViewError", err, err)
	}
	if !strings.Contains(viewErr.Reason, "does not carry") {
		t.Errorf("Reason = %q, want it to explain the target is absent", viewErr.Reason)
	}
}

func TestE2EExcludedAssetIsNotInstalled(t *testing.T) {
	env := newPatchEnv(t, "personas/dropped.md")
	env.writeUpstream("personas/kept.md", "---\nname: kept\ndescription: A kept persona\n---\n\nOriginal body.\n")
	env.writeUpstream("personas/dropped.md", "---\nname: dropped\ndescription: Dropped\n---\n\nDropped\n")
	env.SetupBundle("test", "name: test\nassets:\n  - type: persona\n    path: upstream:personas/kept.md\n")
	// A patch is declared but targets nothing excluded, so the view still builds.
	env.writePatch("noop", `version: 1
file: personas/kept.md
edits:
  - find: Original body.
    replace: Patched body.
`)

	if _, err := env.installOnce(t); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if !env.TargetFileExists("output-styles/kept.md") {
		t.Error("non-excluded asset should install")
	}
	if env.TargetFileExists("output-styles/dropped.md") {
		t.Error("excluded asset must not install")
	}

	// The excluded file must not reach the materialized view either.
	sources, err := env.loader.LoadAll(env.taiCfg.Sources)
	if err != nil {
		t.Fatal(err)
	}
	up := sources["upstream"]
	patches, err := patch.LoadDir(filepath.Join(env.ConfigRepo, "patches", "upstream"), "upstream")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.loader.BuildView(up, patches); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(up.ViewPath, "personas", "dropped.md")); !os.IsNotExist(err) {
		t.Error("excluded file was copied into the view")
	}
}

func TestE2EReferencingAnExcludedAssetExplainsWhy(t *testing.T) {
	env := newPatchEnv(t, "personas/dropped.md")
	env.writeUpstream("personas/dropped.md", "---\nname: dropped\ndescription: Dropped\n---\n\nDropped\n")
	env.SetupBundle("test", "name: test\nassets:\n  - type: persona\n    path: upstream:personas/dropped.md\n")

	_, err := env.installOnce(t)
	if err == nil {
		t.Fatal("expected an error referencing an excluded asset")
	}
	if !strings.Contains(err.Error(), "excluded by source") {
		t.Errorf("err = %v, want it to name the exclusion rather than report 'not found'", err)
	}
}
