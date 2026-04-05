package install

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/gemini"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func setupGeminiInstallTest(t *testing.T) (*testutil.TestEnv, *Installer, *gemini.Adapter) {
	t.Helper()
	env := testutil.NewTestEnv(t)
	manifestStore := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := gemini.New(env.TargetDir)
	return env, installer, adapter
}

func TestInstallGeminiPersonaMappedToSkill(t *testing.T) {
	env, installer, adapter := setupGeminiInstallTest(t)

	env.SetupAsset("personas", "senior-swe.md", `---
tai-schema-version: 1
name: Senior SWE
description: A senior software engineer persona
---

You are a senior software engineer.
`)
	env.SetupAsset("skills", "commit.md", `---
tai-schema-version: 1
name: commit
description: Create commits
tools: Read, Edit
---

Commit instructions.
`)

	assets := []*config.Asset{
		{
			Type:       config.AssetTypePersona,
			Name:       "senior-swe.md",
			SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "senior-swe.md"),
		},
		{
			Type:       config.AssetTypeSkill,
			Name:       "commit.md",
			SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "skills", "commit.md"),
		},
	}

	result, err := installer.Install(adapter, "gemini-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Persona should be wrapped in a directory for Gemini
	if env.TargetFileExists("personas/senior-swe.md") {
		t.Error("persona should NOT be installed to personas/ for Gemini")
	}
	if env.TargetFileExists("skills/senior-swe.md") {
		t.Error("persona should NOT be a loose file in skills/")
	}
	if !env.TargetFileExists("skills/senior-swe/SKILL.md") {
		t.Fatal("persona should be installed to skills/senior-swe/SKILL.md")
	}

	// Single-file skill should also be wrapped in a directory
	if env.TargetFileExists("skills/commit.md") {
		t.Error("single-file skill should NOT be a loose file in skills/")
	}
	if !env.TargetFileExists("skills/commit/SKILL.md") {
		t.Fatal("single-file skill should be installed to skills/commit/SKILL.md")
	}

	// Persona warning about skill mapping
	if !containsWarning(result.Warnings, "installed as skill instead: skills/senior-swe/SKILL.md") {
		t.Errorf("missing persona-to-skill mapping warning, warnings: %#v", result.Warnings)
	}

	// Persona content should have name+description frontmatter (like a skill)
	persona := env.ReadTargetFile("skills/senior-swe/SKILL.md")
	if !strings.Contains(persona, "name: Senior SWE") {
		t.Errorf("persona-as-skill missing name field: %q", persona)
	}
	if !strings.Contains(persona, "description: A senior software engineer persona") {
		t.Errorf("persona-as-skill missing description field: %q", persona)
	}

	// Skill should have name+description but NOT tools (Gemini uses minimal frontmatter)
	skill := env.ReadTargetFile("skills/commit/SKILL.md")
	if !strings.Contains(skill, "name: commit") {
		t.Errorf("skill missing name frontmatter: %q", skill)
	}
	if strings.Contains(skill, "tools:") {
		t.Errorf("skill should not include tools frontmatter for Gemini: %q", skill)
	}
}

func TestInstallGeminiDelegateSkipped(t *testing.T) {
	env, installer, adapter := setupGeminiInstallTest(t)

	env.SetupAsset("delegates", "reviewer.md", `---
tai-schema-version: 1
name: code-reviewer
description: Reviews code
tools: Read, Glob
---

You are a code reviewer.
`)

	assets := []*config.Asset{
		{
			Type:       config.AssetTypeDelegate,
			Name:       "reviewer.md",
			SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "reviewer.md"),
		},
	}

	result, err := installer.Install(adapter, "gemini-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Delegate should NOT be installed anywhere
	if env.TargetFileExists("delegates/reviewer.md") {
		t.Error("delegate should not be installed for Gemini")
	}
	if env.TargetFileExists("skills/reviewer.md") {
		t.Error("delegate should not be mapped to skills for Gemini")
	}
	if env.TargetFileExists("agents/reviewer.md") {
		t.Error("delegate should not be mapped to agents for Gemini")
	}

	// Should have a skip warning
	if !containsWarning(result.Warnings, "not supported by target 'gemini'") {
		t.Errorf("missing delegate skip warning, warnings: %#v", result.Warnings)
	}

	// No files should have been added
	if len(result.Added) != 0 {
		t.Errorf("Added = %#v, want empty", result.Added)
	}
}

func TestInstallGeminiSkipsUnsupportedStatusline(t *testing.T) {
	env, installer, adapter := setupGeminiInstallTest(t)

	env.SetupAsset("statuslines", "s.yaml", "level: minimal\n")
	assets := []*config.Asset{
		{
			Type:       config.AssetTypeStatusLine,
			Name:       "s.yaml",
			SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "s.yaml"),
		},
	}

	result, err := installer.Install(adapter, "gemini-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !containsWarning(result.Warnings, "Asset type 'statusline' is not supported by target 'gemini'") {
		t.Errorf("missing unsupported statusline warning, warnings: %#v", result.Warnings)
	}

	if len(result.Added) != 0 {
		t.Errorf("Added = %#v, want empty", result.Added)
	}
}
