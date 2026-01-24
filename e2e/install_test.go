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

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
default_profile: work
`)

	env.SetupAsset("personas", "senior-swe.md", `---
tai-schema-version: 1
name: Senior SWE
description: A senior software engineer persona
---

# Senior Software Engineer

You are a senior software engineer.
`)

	env.SetupAsset("delegates", "code-reviewer.md", `---
tai-schema-version: 1
name: code-reviewer
description: Reviews code for quality
tools: Read, Glob, Grep
---

You are a code reviewer.
`)

	env.SetupAsset("docs", "CLAUDE.md", `---
tai-schema-version: 1
---

# Project Instructions

These are the project instructions.
`)

	env.SetupBundle("core", `name: core
description: Core assets for work
assets:
  - type: persona
    path: personas/senior-swe.md
  - type: delegate
    path: delegates/code-reviewer.md
  - type: doc
    path: docs/CLAUDE.md
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
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := claude.New(env.TargetDir)

	// Resolve bundle assets manually for the test
	bundle := bundles["core"]
	var assets []*config.Asset
	for _, entry := range bundle.Assets {
		sourceName, assetPath := config.ParseAssetReference(entry.Path)
		src := sources[sourceName]
		asset, _ := src.ResolveAsset(assetPath, entry.Type)
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

	// Verify persona content has Claude frontmatter (name + description, no tai-schema-version)
	personaContent := env.ReadTargetFile("output-styles/senior-swe.md")
	expectedPersona := "---\nname: Senior SWE\ndescription: A senior software engineer persona\n---\n\n# Senior Software Engineer\n\nYou are a senior software engineer.\n"
	if personaContent != expectedPersona {
		t.Errorf("persona content = %q, want %q", personaContent, expectedPersona)
	}

	// Verify delegate content has Claude agent frontmatter (name + description + tools)
	delegateContent := env.ReadTargetFile("agents/code-reviewer.md")
	expectedDelegate := "---\nname: code-reviewer\ndescription: Reviews code for quality\ntools: Read, Glob, Grep\n---\n\nYou are a code reviewer.\n"
	if delegateContent != expectedDelegate {
		t.Errorf("delegate content = %q, want %q", delegateContent, expectedDelegate)
	}

	// Verify doc content is body only (tai frontmatter stripped, no target frontmatter)
	docContent := env.ReadTargetFile("CLAUDE.md")
	expectedDoc := "# Project Instructions\n\nThese are the project instructions.\n"
	if docContent != expectedDoc {
		t.Errorf("doc content = %q, want %q", docContent, expectedDoc)
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

	env.SetupAsset("personas", "original.md", "---\ntai-schema-version: 1\nname: Original\n---\n\n# Original")
	env.SetupBundle("v1", `name: v1
assets:
  - type: persona
    path: personas/original.md
`)
	env.SetupProfile("test", `name: test
target: claude
bundles:
  - v1
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := claude.New(env.TargetDir)

	asset, _ := sources["local"].ResolveAsset("personas/original.md", config.AssetTypePersona)
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Now add a new asset and update bundle
	env.SetupAsset("personas", "new.md", "---\ntai-schema-version: 1\nname: New\n---\n\n# New")
	env.SetupBundle("v1", `name: v1
assets:
  - type: persona
    path: personas/new.md
`)

	sources, _ = loader.LoadAll(taiConfig.Sources)

	newAsset, _ := sources["local"].ResolveAsset("personas/new.md", config.AssetTypePersona)
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

	if env.TargetFileExists("output-styles/original.md") {
		t.Error("original.md should have been removed")
	}
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

	env.SetupAsset("personas", "templated.md", "---\ntai-schema-version: 1\nname: Templated\n---\n\n# Project: {{project_name}}")
	env.SetupBundle("test", `name: test
assets:
  - type: persona
    path: personas/templated.md
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

	asset, _ := sources["local"].ResolveAsset("personas/templated.md", config.AssetTypePersona)
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{
		Variables: template.Variables{"project_name": "MyApp"},
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("output-styles/templated.md")
	expected := "---\nname: Templated\n---\n\n# Project: MyApp"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

// TestE2EInstallWithExisting tests {{existing}} templating.
func TestE2EInstallWithExisting(t *testing.T) {
	env := testutil.NewTestEnv(t)

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

	env.SetupAsset("docs", "CLAUDE.md", "---\ntai-schema-version: 1\n---\n\n# Header\n\n{{existing}}\n\n# Footer")
	env.SetupBundle("test", `name: test
assets:
  - type: doc
    path: docs/CLAUDE.md
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

	asset, _ := sources["local"].ResolveAsset("docs/CLAUDE.md", config.AssetTypeDoc)
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("CLAUDE.md")
	expected := "# Header\n\n# User Content\n\n# Footer"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}

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

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ntai-schema-version: 1\nname: my-skill\ndescription: A test skill\n---\n\n# My Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.sh"), []byte("#!/bin/bash\necho test"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupBundle("test", `name: test
assets:
  - type: skill
    path: skills/my-skill
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

	asset, _ := sources["local"].ResolveAsset("skills/my-skill", config.AssetTypeSkill)
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !env.TargetFileExists("skills/my-skill/SKILL.md") {
		t.Error("SKILL.md not copied")
	}
	if !env.TargetFileExists("skills/my-skill/helper.sh") {
		t.Error("helper.sh not copied")
	}

	// SKILL.md should be transformed (tai-schema-version stripped, target frontmatter added)
	skillContent := env.ReadTargetFile("skills/my-skill/SKILL.md")
	expectedSkill := "---\nname: my-skill\ndescription: A test skill\n---\n\n# My Skill\n"
	if skillContent != expectedSkill {
		t.Errorf("SKILL.md content = %q, want %q", skillContent, expectedSkill)
	}
}

// TestE2EInstallSkillWithIncludes tests installing a skill with tai.yaml includes.
func TestE2EInstallSkillWithIncludes(t *testing.T) {
	env := testutil.NewTestEnv(t)

	radSource := filepath.Join(env.Root, "rad-source")
	if err := os.MkdirAll(radSource, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(radSource, "SYNTAX.md"), []byte("# Rad Syntax Reference"), 0644); err != nil {
		t.Fatal(err)
	}

	examplesDir := filepath.Join(radSource, "examples")
	if err := os.MkdirAll(examplesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(examplesDir, "hello.rad"), []byte("print 'Hello, World!'"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: rad
    path: ` + radSource + `
`)

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "rad")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ntai-schema-version: 1\nname: rad\ndescription: Write and debug Rad scripts\n---\n\n# Rad Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}

	manifestContent := `include:
  - from: rad
    path: SYNTAX.md
    dest: references/
  - from: rad
    path: examples
    dest: scripts/
`
	if err := os.WriteFile(filepath.Join(skillDir, "tai.yaml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupBundle("test", `name: test
assets:
  - type: skill
    path: skills/rad
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
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, sources)
	adapter := claude.New(env.TargetDir)

	asset, _ := sources["local"].ResolveAsset("skills/rad", config.AssetTypeSkill)
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !env.TargetFileExists("skills/rad/SKILL.md") {
		t.Error("SKILL.md not copied")
	}

	if env.TargetFileExists("skills/rad/tai.yaml") {
		t.Error("tai.yaml should NOT be copied to target")
	}

	if !env.TargetFileExists("skills/rad/references/SYNTAX.md") {
		t.Error("SYNTAX.md include not resolved")
	}
	syntaxContent := env.ReadTargetFile("skills/rad/references/SYNTAX.md")
	if syntaxContent != "# Rad Syntax Reference" {
		t.Errorf("SYNTAX.md content = %q, want '# Rad Syntax Reference'", syntaxContent)
	}

	if !env.TargetFileExists("skills/rad/scripts/examples/hello.rad") {
		t.Error("examples directory include not resolved")
	}
	helloContent := env.ReadTargetFile("skills/rad/scripts/examples/hello.rad")
	if helloContent != "print 'Hello, World!'" {
		t.Errorf("hello.rad content = %q, want \"print 'Hello, World!'\"", helloContent)
	}

	// Verify SKILL.md was transformed
	skillContent := env.ReadTargetFile("skills/rad/SKILL.md")
	expectedSkill := "---\nname: rad\ndescription: Write and debug Rad scripts\n---\n\n# Rad Skill\n"
	if skillContent != expectedSkill {
		t.Errorf("SKILL.md content = %q, want %q", skillContent, expectedSkill)
	}
}

// TestE2EInstallSkillWithIncludesMissingSource tests error handling for missing include source.
func TestE2EInstallSkillWithIncludesMissingSource(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "bad-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ntai-schema-version: 1\nname: Bad Skill\n---\n\n# Bad Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	manifestContent := `include:
  - from: nonexistent
    path: file.md
    dest: docs/
`
	if err := os.WriteFile(filepath.Join(skillDir, "tai.yaml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupBundle("test", `name: test
assets:
  - type: skill
    path: skills/bad-skill
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
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, sources)
	adapter := claude.New(env.TargetDir)

	asset, _ := sources["local"].ResolveAsset("skills/bad-skill", config.AssetTypeSkill)
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})

	if err == nil {
		t.Error("Install should fail when include references non-existent source")
	}
}
