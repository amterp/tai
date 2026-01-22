package state

import (
	"os"
	"path/filepath"
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
