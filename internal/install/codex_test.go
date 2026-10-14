package install

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/codex"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func setupCodexInstallTest(t *testing.T) (*testutil.TestEnv, *Installer, *codex.Adapter) {
	t.Helper()
	env := testutil.NewTestEnv(t)
	manifestStore := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := codex.New(env.TargetDir)
	return env, installer, adapter
}

func TestInstallCodexMappingsAndWarnings(t *testing.T) {
	env, installer, adapter := setupCodexInstallTest(t)

	env.SetupAsset("personas", "p.md", `---
tai-schema-version: 1
name: Persona A
description: Persona description
---

Persona body.
`)
	env.SetupAsset("delegates", "d.md", `---
tai-schema-version: 1
name: delegate-a
description: Delegate description
tools: Read, Edit
---

Delegate body.
`)
	env.SetupAsset("skills", "s.md", `---
tai-schema-version: 1
name: skill-a
description: Skill description
tools: Read, Edit
---

Skill body.
`)

	assets := []*config.Asset{
		{
			Type:       config.AssetTypePersona,
			Name:       "p.md",
			SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "p.md"),
		},
		{
			Type:       config.AssetTypeDelegate,
			Name:       "d.md",
			SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "d.md"),
		},
		{
			Type:       config.AssetTypeSkill,
			Name:       "s.md",
			SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "skills", "s.md"),
		},
	}

	result, err := installer.Install(adapter, "codex-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Personas are wrapped as skills/<name>/SKILL.md since Codex has no persona concept.
	if !env.TargetFileExists("skills/p/SKILL.md") {
		t.Fatal("skills/p/SKILL.md not created")
	}
	if !env.TargetFileExists("agents/d.toml") {
		t.Fatal("agents/d.toml not created")
	}
	if !env.TargetFileExists("skills/s/SKILL.md") {
		t.Fatal("skills/s/SKILL.md not created")
	}

	if !containsWarning(result.Warnings, "installed as skill instead") {
		t.Errorf("missing persona-as-skill warning, warnings: %#v", result.Warnings)
	}
	// The adapter drops tools from a Codex agent, and that has to reach the
	// user rather than stay on the resolved asset.
	if !containsWarning(result.Warnings, "no tool allowlist") {
		t.Errorf("missing delegate tools warning, warnings: %#v", result.Warnings)
	}

	persona := env.ReadTargetFile("skills/p/SKILL.md")
	if !strings.Contains(persona, "name: Persona A") {
		t.Errorf("persona missing rendered name field: %q", persona)
	}

	delegate := env.ReadTargetFile("agents/d.toml")
	if !strings.Contains(delegate, "name = 'delegate-a'") || !strings.Contains(delegate, "Delegate body.") {
		t.Errorf("delegate not rendered as a Codex agent: %q", delegate)
	}

	skill := env.ReadTargetFile("skills/s/SKILL.md")
	if !strings.Contains(skill, "name: skill-a") {
		t.Errorf("skill missing name frontmatter: %q", skill)
	}
	if strings.Contains(skill, "tools:") {
		t.Errorf("skill should not include tools frontmatter for codex: %q", skill)
	}
}

func TestInstallCodexRemovesDelegateFromOldLayout(t *testing.T) {
	// Delegates used to install as passive files under delegates/. An install
	// recorded that way must have its file swept on the next install, or the
	// old copy lingers next to the agent that replaced it.
	env, installer, adapter := setupCodexInstallTest(t)

	content := "---\nname: delegate-a\ndescription: D\n---\n\nBody.\n"
	env.SetupAsset("delegates", "d.md", content)
	assets := []*config.Asset{{
		Type:       config.AssetTypeDelegate,
		Name:       "d.md",
		SourceName: "local",
		SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "d.md"),
	}}

	env.WriteTargetFile("delegates/d.md", content)
	store := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	if err := store.Save(BuildManifest("codex", "codex-profile", env.ConfigRepo, []ManifestEntry{{
		Path:       "delegates/d.md",
		SourceName: "local",
		SourcePath: assets[0].SourcePath,
		Checksum:   ComputeChecksum([]byte(content)),
	}}, nil)); err != nil {
		t.Fatalf("saving old-layout manifest: %v", err)
	}

	if _, err := installer.Install(adapter, "codex-profile", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if env.TargetFileExists("delegates/d.md") {
		t.Error("delegates/d.md from the old layout was not removed")
	}
	if !env.TargetFileExists("agents/d.toml") {
		t.Error("agents/d.toml not created")
	}
}

func TestInstallCodexSkipsUnsupportedStatusline(t *testing.T) {
	env, installer, adapter := setupCodexInstallTest(t)

	env.SetupAsset("statuslines", "s.yaml", "level: minimal\n")
	assets := []*config.Asset{
		{
			Type:       config.AssetTypeStatusLine,
			Name:       "s.yaml",
			SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", "s.yaml"),
		},
	}

	result, err := installer.Install(adapter, "codex-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !containsWarning(result.Warnings, "Asset type 'statusline' is not supported by target 'codex'") {
		t.Errorf("missing unsupported statusline warning, warnings: %#v", result.Warnings)
	}

	if len(result.Added) != 0 {
		t.Errorf("Added = %#v, want empty", result.Added)
	}
	if len(result.Manifest.ConfigEntries) != 0 {
		t.Errorf("manifest config entries = %d, want 0", len(result.Manifest.ConfigEntries))
	}
}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}
