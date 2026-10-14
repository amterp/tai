package state

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/testutil"
)

func TestLoadGlobalConfig(t *testing.T) {
	env := testutil.NewTestEnv(t)

	paths := &Paths{
		ConfigDir:   env.ConfigDir,
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}

	// No config file yet
	cfg, err := LoadGlobalConfig(paths)
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if cfg.ActiveConfig != "" {
		t.Errorf("ActiveConfig = %q, want empty", cfg.ActiveConfig)
	}

	// Save config
	cfg.ActiveConfig = "/test/path"
	if err := SaveGlobalConfig(paths, cfg); err != nil {
		t.Fatalf("SaveGlobalConfig failed: %v", err)
	}

	// Reload
	cfg2, err := LoadGlobalConfig(paths)
	if err != nil {
		t.Fatalf("LoadGlobalConfig failed: %v", err)
	}
	if cfg2.ActiveConfig != "/test/path" {
		t.Errorf("ActiveConfig = %q, want '/test/path'", cfg2.ActiveConfig)
	}
}

func TestStateSetActiveConfig(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()

	paths := &Paths{
		ConfigDir:   env.ConfigDir,
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}

	state := &State{
		Paths:        paths,
		GlobalConfig: &GlobalConfig{},
	}

	// Set active config
	if err := state.SetActiveConfig(env.ConfigRepo); err != nil {
		t.Fatalf("SetActiveConfig failed: %v", err)
	}

	// Verify it was saved
	if state.GlobalConfig.ActiveConfig != env.ConfigRepo {
		t.Errorf("ActiveConfig = %q, want %q", state.GlobalConfig.ActiveConfig, env.ConfigRepo)
	}

	// Verify config was loaded
	if state.TaiConfig == nil {
		t.Error("TaiConfig is nil")
	}
	if len(state.Sources) == 0 {
		t.Error("Sources is empty")
	}
	if len(state.Bundles) == 0 {
		t.Error("Bundles is empty")
	}
	if len(state.Profiles) == 0 {
		t.Error("Profiles is empty")
	}
}

func TestStateSetActiveConfigInvalid(t *testing.T) {
	env := testutil.NewTestEnv(t)

	paths := &Paths{
		ConfigDir:   env.ConfigDir,
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}

	state := &State{
		Paths:        paths,
		GlobalConfig: &GlobalConfig{},
	}

	// Try to set invalid path
	err := state.SetActiveConfig("/nonexistent/path")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

// writeBrokenRepo writes a config repo whose only source points at a directory
// that does not exist, the way a repo cloned from another machine often does.
func writeBrokenRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := "version: 1\nsources:\n  - name: elsewhere\n    path: ./not-on-this-machine\n"
	if err := os.WriteFile(filepath.Join(dir, "tai.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func testPaths(env *testutil.TestEnv) *Paths {
	return &Paths{
		ConfigDir:   env.ConfigDir,
		CacheDir:    filepath.Join(env.ConfigDir, "cache"),
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}
}

func TestSetActiveConfigRecoversFromBrokenActiveRepo(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()
	paths := testPaths(env)

	broken := writeBrokenRepo(t)
	if err := SaveGlobalConfig(paths, &GlobalConfig{ActiveConfig: broken}); err != nil {
		t.Fatal(err)
	}

	s, err := New(paths, nil)
	if err != nil {
		t.Fatalf("New must not load the active repo, but failed: %v", err)
	}
	if err := s.LoadConfiguredRepo(); err == nil {
		t.Fatal("loading the broken active repo should fail")
	}

	s, err = New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveConfig(env.ConfigRepo); err != nil {
		t.Fatalf("SetActiveConfig away from a broken repo failed: %v", err)
	}
	saved, err := LoadGlobalConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ActiveConfig != env.ConfigRepo {
		t.Errorf("saved ActiveConfig = %q, want %q", saved.ActiveConfig, env.ConfigRepo)
	}
}

func TestSetActiveConfigRefusesRepoThatDoesNotLoad(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()
	paths := testPaths(env)

	if err := SaveGlobalConfig(paths, &GlobalConfig{ActiveConfig: env.ConfigRepo}); err != nil {
		t.Fatal(err)
	}
	s, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}

	broken := writeBrokenRepo(t)
	err = s.SetActiveConfig(broken)
	if err == nil {
		t.Fatal("SetActiveConfig to a repo that does not load should fail")
	}
	if !strings.Contains(err.Error(), env.ConfigRepo) {
		t.Errorf("error should name the unchanged active config %q, got: %v", env.ConfigRepo, err)
	}

	saved, err := LoadGlobalConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ActiveConfig != env.ConfigRepo {
		t.Errorf("config.yaml was rewritten to %q; it should still name %q", saved.ActiveConfig, env.ConfigRepo)
	}
}

func TestResolveOverride(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		flag, env  string
		wantPath   string
		wantOrigin string
	}{
		{"neither", "", "", "", ""},
		{"flag only", "/repo/a", "", "/repo/a", "--config"},
		{"env only", "", "/repo/b", "/repo/b", "TAI_CONFIG"},
		{"flag wins over env", "/repo/a", "/repo/b", "/repo/a", "--config"},
		{"relative made absolute", "repo", "", filepath.Join(cwd, "repo"), "--config"},
		{"home expanded", "", "~/repo", filepath.Join(home, "repo"), "TAI_CONFIG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveOverride(tt.flag, tt.env)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantPath == "" {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
				return
			}
			if got == nil || got.Path != tt.wantPath || got.Origin != tt.wantOrigin {
				t.Errorf("got %+v, want {%s %s}", got, tt.wantPath, tt.wantOrigin)
			}
		})
	}
}

func TestOverrideLoadsInPlaceOfActiveRepo(t *testing.T) {
	env := testutil.NewTestEnv(t)
	paths := testPaths(env)

	// The active repo is broken, so loading it instead of the override fails.
	if err := SaveGlobalConfig(paths, &GlobalConfig{ActiveConfig: writeBrokenRepo(t)}); err != nil {
		t.Fatal(err)
	}
	env.SetupMinimalConfig()

	s, err := New(paths, &RepoOverride{Path: env.ConfigRepo, Origin: "--config"})
	if err != nil {
		t.Fatal(err)
	}
	if s.ConfigRepo() != env.ConfigRepo {
		t.Errorf("ConfigRepo() = %q, want the override %q", s.ConfigRepo(), env.ConfigRepo)
	}
	if err := s.LoadConfiguredRepo(); err != nil {
		t.Fatalf("loading the override repo failed: %v", err)
	}
	if _, ok := s.Profiles["default"]; !ok {
		t.Errorf("override repo's profiles not loaded; got %v", s.Profiles)
	}
	if s.GlobalConfig.ActiveConfig == env.ConfigRepo {
		t.Error("the override must not change the active config")
	}
}

func TestOverrideWithoutTaiYamlNamesItsOrigin(t *testing.T) {
	env := testutil.NewTestEnv(t)
	s, err := New(testPaths(env), &RepoOverride{Path: t.TempDir(), Origin: "TAI_CONFIG"})
	if err != nil {
		t.Fatal(err)
	}
	err = s.LoadConfiguredRepo()
	if err == nil || !strings.Contains(err.Error(), "from TAI_CONFIG") {
		t.Errorf("error should name TAI_CONFIG as the origin, got: %v", err)
	}
}

func TestStateRequireActiveConfig(t *testing.T) {
	state := &State{
		GlobalConfig: &GlobalConfig{},
	}

	// No active config
	err := state.RequireActiveConfig()
	if err == nil {
		t.Error("expected error when no active config, got nil")
	}

	// An override counts as a config repo
	state.Override = &RepoOverride{Path: "/other/path", Origin: "--config"}
	if err := state.RequireActiveConfig(); err != nil {
		t.Errorf("unexpected error with an override: %v", err)
	}
	state.Override = nil

	// With active config
	state.GlobalConfig.ActiveConfig = "/some/path"
	err = state.RequireActiveConfig()
	if err != nil {
		t.Errorf("unexpected error with active config: %v", err)
	}
}

func TestStateLoadConfigRepo(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()

	paths := &Paths{
		ConfigDir:   env.ConfigDir,
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}

	state := &State{
		Paths:        paths,
		GlobalConfig: &GlobalConfig{},
	}

	if err := state.LoadConfigRepo(env.ConfigRepo); err != nil {
		t.Fatalf("LoadConfigRepo failed: %v", err)
	}

	// Verify everything was loaded
	if state.TaiConfig == nil {
		t.Error("TaiConfig is nil")
	}
	if state.Sources == nil {
		t.Error("Sources is nil")
	}
	if state.Bundles == nil {
		t.Error("Bundles is nil")
	}
	if state.Profiles == nil {
		t.Error("Profiles is nil")
	}
	if state.BundleResolver == nil {
		t.Error("BundleResolver is nil")
	}
	if state.ProfileResolver == nil {
		t.Error("ProfileResolver is nil")
	}
	if state.Installer == nil {
		t.Error("Installer is nil")
	}
}

// A profile naming a target tai dropped must say so, not report "unknown
// target" and leave the user looking for a typo.
func TestStateLoadConfigRepoRejectsRemovedTarget(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()
	env.SetupProfile("old", "name: old\ndescription: Old\ntarget: gemini\nbundles:\n  - core\n")

	state := &State{
		Paths: &Paths{
			ConfigDir:   env.ConfigDir,
			ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
			ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
		},
		GlobalConfig: &GlobalConfig{},
	}

	err := state.LoadConfigRepo(env.ConfigRepo)
	if err == nil || !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("LoadConfigRepo error = %v, want one saying gemini is no longer supported", err)
	}
}

// native_frontmatter names a target, and a misspelt one would otherwise
// quietly claim fields for a target that never installs them.
func TestStateLoadConfigRepoRejectsUnknownNativeFrontmatter(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()
	env.SetupTaiConfig("version: 1\nsources:\n  - name: local\n    path: ./assets\n    native_frontmatter: cluade\ndefault_profile: default\n")

	state := &State{
		Paths: &Paths{
			ConfigDir:   env.ConfigDir,
			ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
			ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
		},
		GlobalConfig: &GlobalConfig{},
	}

	err := state.LoadConfigRepo(env.ConfigRepo)
	if err == nil || !strings.Contains(err.Error(), "cluade") || !strings.Contains(err.Error(), "known targets") {
		t.Fatalf("LoadConfigRepo error = %v, want one naming the bad target and listing known ones", err)
	}
}

func TestStateLocateAsset(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()

	paths := &Paths{
		ConfigDir:   env.ConfigDir,
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}

	state := &State{
		Paths:        paths,
		GlobalConfig: &GlobalConfig{ActiveConfig: env.ConfigRepo},
	}

	if err := state.LoadConfigRepo(env.ConfigRepo); err != nil {
		t.Fatalf("LoadConfigRepo failed: %v", err)
	}

	// Find existing asset
	path, err := state.LocateAsset("personas/default.md")
	if err != nil {
		t.Fatalf("LocateAsset failed: %v", err)
	}

	expected := filepath.Join(env.ConfigRepo, "assets", "personas", "default.md")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}

	// Non-existing asset
	_, err = state.LocateAsset("personas/nonexistent.md")
	if err == nil {
		t.Error("expected error for non-existing asset")
	}
}

func TestStateResolveProfile(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()

	paths := &Paths{
		ConfigDir:   env.ConfigDir,
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}

	state := &State{
		Paths:        paths,
		GlobalConfig: &GlobalConfig{ActiveConfig: env.ConfigRepo},
	}

	if err := state.LoadConfigRepo(env.ConfigRepo); err != nil {
		t.Fatalf("LoadConfigRepo failed: %v", err)
	}

	// Resolve existing profile
	resolved, err := state.ResolveProfile("default")
	if err != nil {
		t.Fatalf("ResolveProfile failed: %v", err)
	}
	if resolved.Name != "default" {
		t.Errorf("Name = %q, want 'default'", resolved.Name)
	}
	if resolved.Target != "claude" {
		t.Errorf("Target = %q, want 'claude'", resolved.Target)
	}

	// Non-existing profile
	_, err = state.ResolveProfile("nonexistent")
	if err == nil {
		t.Error("expected error for non-existing profile")
	}
}

func TestStateGetAdapter(t *testing.T) {
	state := &State{}

	// Get claude adapter
	adap, err := state.GetAdapter("claude")
	if err != nil {
		t.Fatalf("GetAdapter failed: %v", err)
	}
	if adap.Name() != "claude" {
		t.Errorf("adapter.Name() = %q, want 'claude'", adap.Name())
	}

	// Get codex adapter
	codexAdap, err := state.GetAdapter("codex")
	if err != nil {
		t.Fatalf("GetAdapter failed: %v", err)
	}
	if codexAdap.Name() != "codex" {
		t.Errorf("adapter.Name() = %q, want 'codex'", codexAdap.Name())
	}

	// Non-existing adapter
	_, err = state.GetAdapter("nonexistent")
	if err == nil {
		t.Error("expected error for non-existing adapter")
	}
}

func TestStateWithHomeTilde(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupMinimalConfig()

	// Create a config repo in home dir simulation
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home directory")
	}

	// Test ~ expansion in paths
	paths := &Paths{
		ConfigDir:   env.ConfigDir,
		ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
		ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
	}

	state := &State{
		Paths:        paths,
		GlobalConfig: &GlobalConfig{},
	}

	// This test just verifies the tilde expansion code path exists
	// We can't easily test it without creating files in home dir
	_ = home
	_ = state
}

func TestStateCommandSources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("command source tests require POSIX sh")
	}

	setup := func(t *testing.T) (*testutil.TestEnv, *State) {
		env := testutil.NewTestEnv(t)
		env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: private
    type: command
    command:
      - sh
      - -c
      - 'mkdir -p "$TAI_DEST_DIR/personas" && printf "# Remote" > "$TAI_DEST_DIR/personas/remote.md"'
`)
		env.SetupAsset("personas", "default.md", "# Default")
		env.SetupBundle("core", `name: core
assets:
  - type: persona
    path: private:personas/remote.md
`)
		env.SetupProfile("default", `name: default
target: claude
bundles:
  - core
`)

		paths := &Paths{
			ConfigDir:   env.ConfigDir,
			CacheDir:    env.CacheDir,
			ManifestDir: filepath.Join(env.ConfigDir, "manifests"),
			ConfigFile:  filepath.Join(env.ConfigDir, "config.yaml"),
		}
		state := &State{
			Paths:        paths,
			GlobalConfig: &GlobalConfig{ActiveConfig: env.ConfigRepo},
		}
		return env, state
	}

	t.Run("load tolerates unfetched command source", func(t *testing.T) {
		_, state := setup(t)
		if err := state.LoadConfigRepo(state.GlobalConfig.ActiveConfig); err != nil {
			t.Fatalf("LoadConfigRepo with unfetched command source failed: %v", err)
		}
		if !state.HasFetchableSources() {
			t.Error("HasFetchableSources() = false, want true")
		}

		// Resolving a profile that references the unfetched source should
		// error with the refresh hint.
		_, err := state.ResolveProfile("default")
		if err == nil {
			t.Fatal("expected error resolving profile against unfetched source")
		}
		if !strings.Contains(err.Error(), "tai refresh") {
			t.Errorf("error should hint at 'tai refresh', got: %v", err)
		}
	})

	t.Run("refresh materializes and rebuilds resolvers", func(t *testing.T) {
		_, state := setup(t)
		if err := state.LoadConfigRepo(state.GlobalConfig.ActiveConfig); err != nil {
			t.Fatal(err)
		}

		var refreshedNames []string
		if err := state.RefreshSources("", func(name string) {
			refreshedNames = append(refreshedNames, name)
		}); err != nil {
			t.Fatalf("RefreshSources() error = %v", err)
		}
		if len(refreshedNames) != 1 || refreshedNames[0] != "private" {
			t.Errorf("refreshed = %v, want [private]", refreshedNames)
		}

		// Post-refresh, the same State must resolve the profile - proving
		// the resolvers were rebuilt against the fresh sources.
		resolved, err := state.ResolveProfile("default")
		if err != nil {
			t.Fatalf("ResolveProfile after refresh: %v", err)
		}
		if len(resolved.Assets) != 1 || resolved.Assets[0].Name != "remote.md" {
			t.Errorf("assets = %+v, want single remote.md", resolved.Assets)
		}

		// LocateAsset should also work now.
		if _, err := state.LocateAsset("private:personas/remote.md"); err != nil {
			t.Errorf("LocateAsset after refresh: %v", err)
		}
	})

	t.Run("refresh of named non-fetchable source rejected", func(t *testing.T) {
		_, state := setup(t)
		if err := state.LoadConfigRepo(state.GlobalConfig.ActiveConfig); err != nil {
			t.Fatal(err)
		}
		if err := state.RefreshSources("local", nil); err == nil {
			t.Error("expected error refreshing a non-fetchable source by name")
		}
		if err := state.RefreshSources("nope", nil); err == nil {
			t.Error("expected error refreshing unknown source")
		}
	})
}

// adapter.TargetNames() is written by hand, because the registry only holds
// adapters the current binary linked in. This package imports all four, so it
// is where the two can be compared.
func TestRegistryMatchesTargetNames(t *testing.T) {
	registered := adapter.DefaultRegistry.List()
	sort.Strings(registered)

	declared := adapter.TargetNames()

	if !slices.Equal(registered, declared) {
		t.Errorf("adapter.TargetNames() = %v, registry has %v - update targetNames in internal/adapter",
			declared, registered)
	}
	for _, name := range declared {
		if _, removed := adapter.RemovedTarget(name); removed {
			t.Errorf("%q is both a supported target and a removed one - drop it from removedTargets", name)
		}
	}
}
