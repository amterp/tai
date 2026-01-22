package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/template"
	"github.com/amterp/tai/internal/testutil"
)

// TestE2EFreshInstall tests installing to a fresh target directory.
func TestE2EFreshInstall(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Set up a complete config repo
	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
default_profile: work
`)

	env.SetupAsset("personas", "senior-swe.md", `---
name: Senior SWE
description: A senior software engineer persona
---

# Senior Software Engineer

You are a senior software engineer.
`)

	env.SetupAsset("delegates", "code-reviewer.md", `---
name: code-reviewer
description: Reviews code for quality
tools: Read, Glob, Grep
---

You are a code reviewer.
`)

	env.SetupAsset("docs", "CLAUDE.md", `# Project Instructions

These are the project instructions.
`)

	env.SetupBundle("core", `name: core
description: Core assets for work
assets:
  - personas/senior-swe.md
  - delegates/code-reviewer.md
  - docs/CLAUDE.md
`)

	env.SetupProfile("work", `name: work
description: Work profile
target: claude
bundles:
  - core
`)

	// Load sources
	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)
	bundles, _ := config.LoadBundlesFromDir(filepath.Join(env.ConfigRepo, "bundles"))

	// Create resolver and installer
	bundleResolver := source.NewLoader(env.ConfigRepo)
	_ = bundleResolver // Not directly used, bundle.NewResolver is used

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	// Resolve bundle assets manually for the test
	bundle := bundles["core"]
	var assets []*config.Asset
	for _, assetRef := range bundle.Assets {
		sourceName, assetPath := config.ParseAssetReference(assetRef)
		src := sources[sourceName]
		asset, _ := src.ResolveAsset(assetPath)
		assets = append(assets, asset)
	}

	// Install
	result, err := installer.Install(adapter, "work", assets, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify results
	if len(result.Added) != 3 {
		t.Errorf("Added = %d, want 3", len(result.Added))
	}

	// Verify files were created with correct mapping
	if !env.TargetFileExists("output-styles/senior-swe.md") {
		t.Error("output-styles/senior-swe.md not created")
	}
	if !env.TargetFileExists("agents/code-reviewer.md") {
		t.Error("agents/code-reviewer.md not created")
	}
	if !env.TargetFileExists("CLAUDE.md") {
		t.Error("CLAUDE.md not created")
	}

	// Verify content
	content := env.ReadTargetFile("output-styles/senior-swe.md")
	if len(content) == 0 {
		t.Error("output-styles/senior-swe.md is empty")
	}

	// Verify manifest was created
	manifest, _ := installer.Status(adapter.Name())
	if manifest == nil {
		t.Fatal("Manifest not created")
	}
	if manifest.Profile != "work" {
		t.Errorf("Manifest.Profile = %q, want 'work'", manifest.Profile)
	}
	if len(manifest.Files) != 3 {
		t.Errorf("Manifest.Files = %d, want 3", len(manifest.Files))
	}
}

// TestE2EReinstallWithChanges tests re-installing with changes.
func TestE2EReinstallWithChanges(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// Initial setup with one asset
	env.SetupAsset("personas", "original.md", "# Original")
	env.SetupBundle("v1", `name: v1
assets:
  - personas/original.md
`)
	env.SetupProfile("test", `name: test
target: claude
bundles:
  - v1
`)

	// Load and install v1
	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	// First install
	asset, _ := sources["local"].ResolveAsset("personas/original.md")
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Now add a new asset and update bundle
	env.SetupAsset("personas", "new.md", "# New")
	env.SetupBundle("v1", `name: v1
assets:
  - personas/new.md
`)

	// Reload sources
	sources, _ = loader.LoadAll(taiConfig.Sources)

	// Second install - should remove original, add new
	newAsset, _ := sources["local"].ResolveAsset("personas/new.md")
	result, err := installer.Install(adapter, "test", []*config.Asset{newAsset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("Added = %d, want 1", len(result.Added))
	}
	if len(result.Removed) != 1 {
		t.Errorf("Removed = %d, want 1", len(result.Removed))
	}

	// Verify old file removed
	if env.TargetFileExists("output-styles/original.md") {
		t.Error("original.md should have been removed")
	}

	// Verify new file created
	if !env.TargetFileExists("output-styles/new.md") {
		t.Error("new.md not created")
	}
}

// TestE2EInstallWithTemplating tests templating during install.
func TestE2EInstallWithTemplating(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// Create asset with template variables
	env.SetupAsset("personas", "templated.md", "# Project: {{project_name}}")
	env.SetupBundle("test", `name: test
assets:
  - personas/templated.md
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
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	asset, _ := sources["local"].ResolveAsset("personas/templated.md")
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{
		Variables: template.Variables{"project_name": "MyApp"},
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("output-styles/templated.md")
	expected := "# Project: MyApp"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

// TestE2EInstallWithExisting tests {{existing}} templating.
func TestE2EInstallWithExisting(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Create existing file in target
	if err := os.MkdirAll(env.TargetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.TargetDir, "CLAUDE.md"), []byte("# User Content"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("docs", "CLAUDE.md", "# Header\n\n{{existing}}\n\n# Footer")
	env.SetupBundle("test", `name: test
assets:
  - docs/CLAUDE.md
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
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	asset, _ := sources["local"].ResolveAsset("docs/CLAUDE.md")
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("CLAUDE.md")
	expected := "# Header\n\n# User Content\n\n# Footer"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}

	// Verify backup was created
	backupPath := filepath.Join(env.TargetDir, "CLAUDE.md.tai-backup")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup not created")
	}
}

// TestE2EInstallSkillDirectory tests installing skill directories.
func TestE2EInstallSkillDirectory(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// Create a skill directory
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: my-skill
description: A test skill
---
# My Skill
`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.sh"), []byte("#!/bin/bash\necho test"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupBundle("test", `name: test
assets:
  - skills/my-skill
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
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	asset, _ := sources["local"].ResolveAsset("skills/my-skill")
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify skill directory was copied
	if !env.TargetFileExists("skills/my-skill/SKILL.md") {
		t.Error("SKILL.md not copied")
	}
	if !env.TargetFileExists("skills/my-skill/helper.sh") {
		t.Error("helper.sh not copied")
	}
}
