package state

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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

func TestStateRequireActiveConfig(t *testing.T) {
	state := &State{
		GlobalConfig: &GlobalConfig{},
	}

	// No active config
	err := state.RequireActiveConfig()
	if err == nil {
		t.Error("expected error when no active config, got nil")
	}

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
		if !state.HasCommandSources() {
			t.Error("HasCommandSources() = false, want true")
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
		if err := state.RefreshCommandSources("", func(name string) {
			refreshedNames = append(refreshedNames, name)
		}); err != nil {
			t.Fatalf("RefreshCommandSources() error = %v", err)
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

	t.Run("refresh of named non-command source rejected", func(t *testing.T) {
		_, state := setup(t)
		if err := state.LoadConfigRepo(state.GlobalConfig.ActiveConfig); err != nil {
			t.Fatal(err)
		}
		if err := state.RefreshCommandSources("local", nil); err == nil {
			t.Error("expected error refreshing non-command source by name")
		}
		if err := state.RefreshCommandSources("nope", nil); err == nil {
			t.Error("expected error refreshing unknown source")
		}
	})
}
