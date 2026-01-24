package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/testutil"
)

// TestE2EProfileSwitching tests switching between profiles.
func TestE2EProfileSwitching(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("personas", "personal.md", "---\ntai-schema-version: 1\nname: Personal\n---\n\n# Personal Persona")
	env.SetupAsset("personas", "work.md", "---\ntai-schema-version: 1\nname: Work\n---\n\n# Work Persona")
	env.SetupAsset("personas", "shared.md", "---\ntai-schema-version: 1\nname: Shared\n---\n\n# Shared Persona")

	env.SetupBundle("personal-bundle", `name: personal-bundle
assets:
  - type: persona
    path: personas/personal.md
  - type: persona
    path: personas/shared.md
`)

	env.SetupBundle("work-bundle", `name: work-bundle
assets:
  - type: persona
    path: personas/work.md
  - type: persona
    path: personas/shared.md
`)

	env.SetupProfile("personal", `name: personal
target: claude
bundles:
  - personal-bundle
`)

	env.SetupProfile("work", `name: work
target: claude
bundles:
  - work-bundle
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := claude.New(env.TargetDir)

	// Install personal profile
	personalAsset, _ := sources["local"].ResolveAsset("personas/personal.md", config.AssetTypePersona)
	sharedAsset, _ := sources["local"].ResolveAsset("personas/shared.md", config.AssetTypePersona)
	_, err := installer.Install(adapter, "personal", []*config.Asset{personalAsset, sharedAsset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install personal failed: %v", err)
	}

	if !env.TargetFileExists("output-styles/personal.md") {
		t.Error("personal.md not installed")
	}
	if !env.TargetFileExists("output-styles/shared.md") {
		t.Error("shared.md not installed")
	}

	// Switch to work profile
	workAsset, _ := sources["local"].ResolveAsset("personas/work.md", config.AssetTypePersona)
	result, err := installer.Install(adapter, "work", []*config.Asset{workAsset, sharedAsset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install work failed: %v", err)
	}

	if len(result.Removed) != 1 {
		t.Errorf("Removed = %d, want 1", len(result.Removed))
	}
	if len(result.Added) != 1 {
		t.Errorf("Added = %d, want 1", len(result.Added))
	}

	if env.TargetFileExists("output-styles/personal.md") {
		t.Error("personal.md should be removed")
	}
	if !env.TargetFileExists("output-styles/work.md") {
		t.Error("work.md not installed")
	}
	if !env.TargetFileExists("output-styles/shared.md") {
		t.Error("shared.md should still exist")
	}

	manifest, _ := installer.Status(adapter.Name())
	if manifest.Profile != "work" {
		t.Errorf("Profile = %q, want 'work'", manifest.Profile)
	}
}

// TestE2EConflictResolution tests conflict handling options.
func TestE2EConflictResolution(t *testing.T) {
	env := testutil.NewTestEnv(t)

	targetStylesDir := filepath.Join(env.TargetDir, "output-styles")
	if err := os.MkdirAll(targetStylesDir, 0755); err != nil {
		t.Fatal(err)
	}
	conflictPath := filepath.Join(targetStylesDir, "conflict.md")
	if err := os.WriteFile(conflictPath, []byte("# Existing Unmanaged"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("personas", "conflict.md", "---\ntai-schema-version: 1\nname: Conflict\n---\n\n# New Content")
	env.SetupAsset("personas", "noconflict.md", "---\ntai-schema-version: 1\nname: Noconflict\n---\n\n# No Conflict")
	env.SetupBundle("test", `name: test
assets:
  - type: persona
    path: personas/conflict.md
  - type: persona
    path: personas/noconflict.md
`)
	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := claude.New(env.TargetDir)

	conflictAsset, _ := sources["local"].ResolveAsset("personas/conflict.md", config.AssetTypePersona)
	noconflictAsset, _ := sources["local"].ResolveAsset("personas/noconflict.md", config.AssetTypePersona)
	assets := []*config.Asset{conflictAsset, noconflictAsset}

	// Test 1: Without flags, should fail
	_, err := installer.Install(adapter, "test", assets, install.InstallOptions{})
	if err == nil {
		t.Error("Expected conflict error, got nil")
	}

	// Test 2: With skip-conflicts, should skip conflicting file
	result, err := installer.Install(adapter, "test", assets, install.InstallOptions{SkipConflicts: true})
	if err != nil {
		t.Fatalf("Install with skip-conflicts failed: %v", err)
	}

	if len(result.Skipped) != 1 {
		t.Errorf("Skipped = %d, want 1", len(result.Skipped))
	}
	if len(result.Added) != 1 {
		t.Errorf("Added = %d, want 1", len(result.Added))
	}

	content, _ := os.ReadFile(conflictPath)
	if string(content) != "# Existing Unmanaged" {
		t.Error("Conflict file was overwritten with skip-conflicts")
	}

	// Clean up for next test
	if _, err := installer.Uninstall(adapter); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if err := os.WriteFile(conflictPath, []byte("# Existing Unmanaged"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Test 3: With force, should overwrite
	result, err = installer.Install(adapter, "test", assets, install.InstallOptions{Force: true})
	if err != nil {
		t.Fatalf("Install with force failed: %v", err)
	}

	if len(result.Added) != 2 {
		t.Errorf("Added = %d, want 2", len(result.Added))
	}

	content, _ = os.ReadFile(conflictPath)
	expectedContent := "---\nname: Conflict\n---\n\n# New Content"
	if string(content) != expectedContent {
		t.Errorf("Conflict file not overwritten with force: %s", string(content))
	}
}

// TestE2EDryRun tests that dry-run doesn't modify anything.
func TestE2EDryRun(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test")
	env.SetupBundle("test", `name: test
assets:
  - type: persona
    path: personas/test.md
`)
	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := claude.New(env.TargetDir)

	asset, _ := sources["local"].ResolveAsset("personas/test.md", config.AssetTypePersona)
	result, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Dry run failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("Added = %d, want 1", len(result.Added))
	}

	if env.TargetFileExists("output-styles/test.md") {
		t.Error("File created during dry run")
	}

	manifest, _ := installer.Status(adapter.Name())
	if manifest != nil {
		t.Error("Manifest created during dry run")
	}
}

// TestE2EMultipleSources tests using multiple sources.
func TestE2EMultipleSources(t *testing.T) {
	env := testutil.NewTestEnv(t)

	secondaryPath := filepath.Join(env.Root, "secondary")
	secondaryAssets := filepath.Join(secondaryPath, "personas")
	if err := os.MkdirAll(secondaryAssets, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondaryAssets, "secondary.md"), []byte("---\ntai-schema-version: 1\nname: Secondary\n---\n\n# Secondary"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: secondary
    path: ` + secondaryPath + `
`)

	env.SetupAsset("personas", "local.md", "---\ntai-schema-version: 1\nname: Local\n---\n\n# Local")
	env.SetupBundle("combined", `name: combined
assets:
  - type: persona
    path: personas/local.md
  - type: persona
    path: secondary:personas/secondary.md
`)
	env.SetupProfile("test", `name: test
target: claude
bundles:
  - combined
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, err := loader.LoadAll(taiConfig.Sources)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := claude.New(env.TargetDir)

	localAsset, _ := sources["local"].ResolveAsset("personas/local.md", config.AssetTypePersona)
	secondaryAsset, _ := sources["secondary"].ResolveAsset("personas/secondary.md", config.AssetTypePersona)

	_, err = installer.Install(adapter, "test", []*config.Asset{localAsset, secondaryAsset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !env.TargetFileExists("output-styles/local.md") {
		t.Error("local.md not installed")
	}
	if !env.TargetFileExists("output-styles/secondary.md") {
		t.Error("secondary.md not installed")
	}

	manifest, _ := installer.Status(adapter.Name())
	sources_found := make(map[string]bool)
	for _, f := range manifest.Files {
		sources_found[f.SourceName] = true
	}
	if !sources_found["local"] {
		t.Error("local source not tracked in manifest")
	}
	if !sources_found["secondary"] {
		t.Error("secondary source not tracked in manifest")
	}
}
