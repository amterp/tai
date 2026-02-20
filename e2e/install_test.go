package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/profile"
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

	env.SetupAsset("files", "CLAUDE.md", `# Project Instructions

These are the project instructions.
`)

	env.SetupBundle("core", `name: core
description: Core assets for work
assets:
  - type: persona
    path: personas/senior-swe.md
  - type: delegate
    path: delegates/code-reviewer.md
  - type: file
    path: files/CLAUDE.md
    dest: ""
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
		asset.Dest = entry.Dest // Propagate dest for file type
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

	// Verify file content is copied as-is (no frontmatter processing)
	fileContent := env.ReadTargetFile("CLAUDE.md")
	expectedFile := "# Project Instructions\n\nThese are the project instructions.\n"
	if fileContent != expectedFile {
		t.Errorf("file content = %q, want %q", fileContent, expectedFile)
	}

	// Verify manifest was created
	manifest, _ := installer.Status(adapter.Name())
	if manifest == nil {
		t.Fatal("Manifest not created")
		return
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

	env.SetupAsset("files", "CLAUDE.md", "# Header\n\n{{existing}}\n\n# Footer")
	env.SetupBundle("test", `name: test
assets:
  - type: file
    path: files/CLAUDE.md
    dest: ""
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

	asset, _ := sources["local"].ResolveAsset("files/CLAUDE.md", config.AssetTypeFile)
	asset.Dest = "" // Root level
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

// TestE2EInstallNoSchemaVersion tests that assets without tai-schema-version install correctly.
func TestE2EInstallNoSchemaVersion(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// Assets without tai-schema-version
	env.SetupAsset("personas", "modern.md", `---
name: Modern Persona
description: No schema version needed
---

# Modern Persona

Works without tai-schema-version.
`)

	env.SetupAsset("files", "bare.md", `# Bare Doc

No frontmatter at all.
`)

	env.SetupBundle("test", `name: test
assets:
  - type: persona
    path: personas/modern.md
  - type: file
    path: files/bare.md
    dest: ""
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

	var assets []*config.Asset
	for _, entry := range []struct {
		path     string
		assetTyp string
		dest     string
	}{
		{"personas/modern.md", config.AssetTypePersona, ""},
		{"files/bare.md", config.AssetTypeFile, ""},
	} {
		asset, err := sources["local"].ResolveAsset(entry.path, entry.assetTyp)
		if err != nil {
			t.Fatalf("ResolveAsset(%s): %v", entry.path, err)
		}
		asset.Dest = entry.dest
		assets = append(assets, asset)
	}

	result, err := installer.Install(adapter, "test", assets, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 2 {
		t.Errorf("Added = %d, want 2", len(result.Added))
	}

	// Persona gets target frontmatter without tai-schema-version
	personaContent := env.ReadTargetFile("output-styles/modern.md")
	expectedPersona := "---\nname: Modern Persona\ndescription: No schema version needed\n---\n\n# Modern Persona\n\nWorks without tai-schema-version.\n"
	if personaContent != expectedPersona {
		t.Errorf("persona content = %q, want %q", personaContent, expectedPersona)
	}

	// File asset with no frontmatter — copied as-is
	fileContent := env.ReadTargetFile("bare.md")
	expectedFile := "# Bare Doc\n\nNo frontmatter at all.\n"
	if fileContent != expectedFile {
		t.Errorf("file content = %q, want %q", fileContent, expectedFile)
	}
}

// TestE2EInstallSkillWithFieldMapping tests skill includes with field remapping.
func TestE2EInstallSkillWithFieldMapping(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// External source with a SKILL.md using non-tai field names
	extSource := filepath.Join(env.Root, "ext-source")
	if err := os.MkdirAll(extSource, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extSource, "SKILL.md"), []byte(`---
title: External Tool
summary: Helps with external things
tools: Read, Bash
---

# External Tool Instructions
`), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: ext
    path: ` + extSource + `
`)

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "ext-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	// tai.yaml with field mapping
	manifestContent := `include:
  - from: ext
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title
      description: summary
`
	if err := os.WriteFile(filepath.Join(skillDir, "tai.yaml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupBundle("test", `name: test
assets:
  - type: skill
    path: skills/ext-skill
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

	asset, _ := sources["local"].ResolveAsset("skills/ext-skill", config.AssetTypeSkill)
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// SKILL.md should have mapped fields: title→name, summary→description
	// "tools" passes through unchanged since it's already a tai field name
	skillContent := env.ReadTargetFile("skills/ext-skill/SKILL.md")
	expectedSkill := "---\nname: External Tool\ndescription: Helps with external things\ntools: Read, Bash\n---\n\n# External Tool Instructions\n"
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

// TestE2EInstallWithDependencies tests that asset dependencies are auto-included.
func TestE2EInstallWithDependencies(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// A skill that depends on two delegates
	env.SetupAsset("skills", "code-review.md", `---
name: Code Review
description: Reviews code with specialized reviewers
depends:
  - delegates/bug-hunter.md
  - delegates/code-reviewer.md
---

# Code Review Skill

Use the bug-hunter and code-reviewer delegates.
`)

	// The delegates it depends on
	env.SetupAsset("delegates", "bug-hunter.md", `---
name: bug-hunter
description: Finds bugs in code
tools: Read, Grep
---

You find bugs.
`)

	env.SetupAsset("delegates", "code-reviewer.md", `---
name: code-reviewer
description: Reviews code for quality
tools: Read, Glob, Grep
---

You review code.
`)

	// Bundle only includes the skill - deps should be auto-included
	env.SetupBundle("review", `name: review
assets:
  - type: skill
    path: skills/code-review.md
`)

	env.SetupProfile("test", `name: test
target: claude
bundles:
  - review
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)
	bundles, _ := config.LoadBundlesFromDir(filepath.Join(env.ConfigRepo, "bundles"))
	profiles, _ := config.LoadProfilesFromDir(filepath.Join(env.ConfigRepo, "profiles"))

	// Use profile resolver which includes dependency resolution
	resolver := profile.NewResolver(profiles, bundles, sources)
	resolved, err := resolver.Resolve("test")
	if err != nil {
		t.Fatalf("Profile resolve failed: %v", err)
	}

	// Should have 3 assets: 2 delegates + 1 skill
	if len(resolved.Assets) != 3 {
		t.Fatalf("len(resolved.Assets) = %d, want 3", len(resolved.Assets))
	}

	// Dependencies should come first (before the skill that needs them)
	assetNames := make([]string, len(resolved.Assets))
	for i, a := range resolved.Assets {
		assetNames[i] = a.Name
	}

	// Skill should be last
	if resolved.Assets[len(resolved.Assets)-1].Name != "code-review.md" {
		t.Errorf("skill should be last, got %v", assetNames)
	}

	// Delegates should be before the skill
	delegateCount := 0
	for i := 0; i < len(resolved.Assets)-1; i++ {
		if resolved.Assets[i].Type == config.AssetTypeDelegate {
			delegateCount++
		}
	}
	if delegateCount != 2 {
		t.Errorf("expected 2 delegates before skill, got %d", delegateCount)
	}

	// Now install and verify files are created
	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, sources)
	adapter := claude.New(env.TargetDir)

	result, err := installer.Install(adapter, "test", resolved.Assets, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 3 {
		t.Errorf("Added = %d, want 3", len(result.Added))
	}

	// Verify all files exist
	if !env.TargetFileExists("skills/code-review.md") {
		t.Error("skills/code-review.md not created")
	}
	if !env.TargetFileExists("agents/bug-hunter.md") {
		t.Error("agents/bug-hunter.md not created")
	}
	if !env.TargetFileExists("agents/code-reviewer.md") {
		t.Error("agents/code-reviewer.md not created")
	}

	// Verify skill content has no depends field
	skillContent := env.ReadTargetFile("skills/code-review.md")
	if strings.Contains(skillContent, "depends") {
		t.Error("skill content should not contain 'depends' field")
	}
}

// TestE2EInstallTransitiveDependencies tests that transitive dependencies are resolved.
func TestE2EInstallTransitiveDependencies(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// A -> B -> C (transitive chain)
	env.SetupAsset("skills", "skill-a.md", `---
name: Skill A
depends:
  - delegates/delegate-b.md
---
A content.
`)

	env.SetupAsset("delegates", "delegate-b.md", `---
name: Delegate B
depends:
  - personas/persona-c.md
---
B content.
`)

	env.SetupAsset("personas", "persona-c.md", `---
name: Persona C
description: The deepest dependency
---
C content.
`)

	env.SetupBundle("test", `name: test
assets:
  - type: skill
    path: skills/skill-a.md
`)

	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)
	bundles, _ := config.LoadBundlesFromDir(filepath.Join(env.ConfigRepo, "bundles"))
	profiles, _ := config.LoadProfilesFromDir(filepath.Join(env.ConfigRepo, "profiles"))

	resolver := profile.NewResolver(profiles, bundles, sources)
	resolved, err := resolver.Resolve("test")
	if err != nil {
		t.Fatalf("Profile resolve failed: %v", err)
	}

	// Should have 3 assets: A, B, C
	if len(resolved.Assets) != 3 {
		t.Fatalf("len(resolved.Assets) = %d, want 3", len(resolved.Assets))
	}

	// Order should be: C first, then B, then A (depth-first)
	if resolved.Assets[0].Name != "persona-c.md" {
		t.Errorf("expected persona-c.md first, got %s", resolved.Assets[0].Name)
	}
	if resolved.Assets[1].Name != "delegate-b.md" {
		t.Errorf("expected delegate-b.md second, got %s", resolved.Assets[1].Name)
	}
	if resolved.Assets[2].Name != "skill-a.md" {
		t.Errorf("expected skill-a.md third, got %s", resolved.Assets[2].Name)
	}
}

// TestE2EInstallDependencyOverride tests explicit assets override dependencies.
func TestE2EInstallDependencyOverride(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// Skill depends on a delegate
	env.SetupAsset("skills", "my-skill.md", `---
name: My Skill
depends:
  - delegates/helper.md
---
Skill content.
`)

	// The delegate
	env.SetupAsset("delegates", "helper.md", `---
name: helper
description: A helper delegate
---
Helper content.
`)

	// Bundle includes BOTH the skill and its dependency explicitly
	env.SetupBundle("test", `name: test
assets:
  - type: skill
    path: skills/my-skill.md
  - type: delegate
    path: delegates/helper.md
`)

	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)
	bundles, _ := config.LoadBundlesFromDir(filepath.Join(env.ConfigRepo, "bundles"))
	profiles, _ := config.LoadProfilesFromDir(filepath.Join(env.ConfigRepo, "profiles"))

	resolver := profile.NewResolver(profiles, bundles, sources)
	resolved, err := resolver.Resolve("test")
	if err != nil {
		t.Fatalf("Profile resolve failed: %v", err)
	}

	// Should still have exactly 2 assets (no duplicate)
	if len(resolved.Assets) != 2 {
		t.Fatalf("len(resolved.Assets) = %d, want 2 (no duplicates)", len(resolved.Assets))
	}

	// The explicit asset should be in the result
	found := false
	for _, a := range resolved.Assets {
		if a.Type == config.AssetTypeDelegate && a.Name == "helper.md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("delegate helper.md should be in resolved assets")
	}
}

// TestE2EInstallDependencyCycleDetection tests that circular dependencies are detected.
func TestE2EInstallDependencyCycleDetection(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// A -> B -> A (cycle)
	env.SetupAsset("delegates", "delegate-a.md", `---
name: Delegate A
depends:
  - delegates/delegate-b.md
---
A content.
`)

	env.SetupAsset("delegates", "delegate-b.md", `---
name: Delegate B
depends:
  - delegates/delegate-a.md
---
B content.
`)

	env.SetupBundle("test", `name: test
assets:
  - type: delegate
    path: delegates/delegate-a.md
`)

	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)
	bundles, _ := config.LoadBundlesFromDir(filepath.Join(env.ConfigRepo, "bundles"))
	profiles, _ := config.LoadProfilesFromDir(filepath.Join(env.ConfigRepo, "profiles"))

	resolver := profile.NewResolver(profiles, bundles, sources)
	_, err := resolver.Resolve("test")

	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("error = %q, should contain 'circular dependency'", err.Error())
	}
}

// TestE2EInstallSkillDirectoryWithDependencies tests skill directories with dependencies.
func TestE2EInstallSkillDirectoryWithDependencies(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	// Create a skill directory
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "review-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	// SKILL.md with dependencies
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: Review Skill
description: A code review skill
depends:
  - delegates/reviewer.md
---

# Review Skill

Use the reviewer delegate.
`), 0644); err != nil {
		t.Fatal(err)
	}

	// A helper file in the skill directory
	if err := os.WriteFile(filepath.Join(skillDir, "prompts.md"), []byte("# Review Prompts"), 0644); err != nil {
		t.Fatal(err)
	}

	// The delegate dependency
	env.SetupAsset("delegates", "reviewer.md", `---
name: reviewer
description: Reviews things
---
Reviewer content.
`)

	env.SetupBundle("test", `name: test
assets:
  - type: skill
    path: skills/review-skill
`)

	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)
	bundles, _ := config.LoadBundlesFromDir(filepath.Join(env.ConfigRepo, "bundles"))
	profiles, _ := config.LoadProfilesFromDir(filepath.Join(env.ConfigRepo, "profiles"))

	resolver := profile.NewResolver(profiles, bundles, sources)
	resolved, err := resolver.Resolve("test")
	if err != nil {
		t.Fatalf("Profile resolve failed: %v", err)
	}

	// Should have 2 assets: delegate + skill directory
	if len(resolved.Assets) != 2 {
		t.Fatalf("len(resolved.Assets) = %d, want 2", len(resolved.Assets))
	}

	// Delegate should come first
	if resolved.Assets[0].Type != config.AssetTypeDelegate {
		t.Errorf("expected delegate first, got %s", resolved.Assets[0].Type)
	}

	// Install and verify
	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo, sources)
	adapter := claude.New(env.TargetDir)

	_, err = installer.Install(adapter, "test", resolved.Assets, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify files exist
	if !env.TargetFileExists("agents/reviewer.md") {
		t.Error("agents/reviewer.md not created")
	}
	if !env.TargetFileExists("skills/review-skill/SKILL.md") {
		t.Error("skills/review-skill/SKILL.md not created")
	}
	if !env.TargetFileExists("skills/review-skill/prompts.md") {
		t.Error("skills/review-skill/prompts.md not created")
	}

	// SKILL.md should not contain depends field
	skillContent := env.ReadTargetFile("skills/review-skill/SKILL.md")
	if strings.Contains(skillContent, "depends") {
		t.Error("SKILL.md should not contain 'depends' field")
	}
}

// TestE2EInstallWithConditionals tests target-conditional content during install.
func TestE2EInstallWithConditionals(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("files", "AGENTS.md", `# Project Instructions

{% if target_name == "codex" %}
## Codex-Specific
This section only appears for Codex.
{% endif %}
## Common Section
This appears for all targets.
`)

	env.SetupBundle("test", `name: test
assets:
  - type: file
    path: files/AGENTS.md
    dest: ""
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

	asset, _ := sources["local"].ResolveAsset("files/AGENTS.md", config.AssetTypeFile)
	asset.Dest = ""
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("AGENTS.md")

	// Claude target: codex-specific section should NOT appear
	if strings.Contains(content, "Codex-Specific") {
		t.Error("codex-specific content should not appear for claude target")
	}
	if !strings.Contains(content, "Common Section") {
		t.Error("common section should appear")
	}
}

// TestE2EInstallWithIncludeAsset tests {% include_asset %} during install.
func TestE2EInstallWithIncludeAsset(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("personas", "senior-swe.md", `---
name: Senior SWE
description: A senior engineer persona
---

You are a senior software engineer.
`)

	env.SetupAsset("files", "AGENTS.md", `# Project
{% include_asset "local:personas/senior-swe.md" %}
Done.
`)

	env.SetupBundle("test", `name: test
assets:
  - type: file
    path: files/AGENTS.md
    dest: ""
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

	asset, _ := sources["local"].ResolveAsset("files/AGENTS.md", config.AssetTypeFile)
	asset.Dest = ""
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("AGENTS.md")

	// The persona body (frontmatter stripped) should be inlined
	if !strings.Contains(content, "You are a senior software engineer.") {
		t.Error("included persona body not found in output")
	}
	if !strings.Contains(content, "# Project") {
		t.Error("outer content not found")
	}
	if !strings.Contains(content, "Done.") {
		t.Error("trailing content not found")
	}
	// Frontmatter should NOT appear in the inlined content
	if strings.Contains(content, "name: Senior SWE") {
		t.Error("frontmatter should be stripped from included asset")
	}
}

// TestE2EInstallConditionalIncludeAsset tests the user's original scenario:
// conditionally including a persona into a file based on target.
func TestE2EInstallConditionalIncludeAsset(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("personas", "company-style.md", `---
name: Company Style
description: Company coding style
---

Follow the company coding standards.
`)

	env.SetupAsset("files", "AGENTS.md", `# Project Instructions

## Core Rules
Always write tests.
{% if target_name == "codex" %}

## Codex Persona
{% include_asset "local:personas/company-style.md" %}{% endif %}
`)

	env.SetupBundle("test", `name: test
assets:
  - type: file
    path: files/AGENTS.md
    dest: ""
`)
	env.SetupProfile("claude-profile", `name: claude-profile
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

	asset, _ := sources["local"].ResolveAsset("files/AGENTS.md", config.AssetTypeFile)
	asset.Dest = ""

	// Install for Claude - should NOT include the persona
	_, err := installer.Install(adapter, "claude-profile", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install for claude failed: %v", err)
	}

	claudeContent := env.ReadTargetFile("AGENTS.md")
	if strings.Contains(claudeContent, "Follow the company coding standards.") {
		t.Error("codex-only persona should not appear for claude target")
	}
	if !strings.Contains(claudeContent, "Always write tests.") {
		t.Error("core rules should appear")
	}
}
