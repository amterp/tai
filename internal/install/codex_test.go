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

func TestInstallCodexMappingsAndPassiveWarnings(t *testing.T) {
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

	if !env.TargetFileExists("personas/p.md") {
		t.Fatal("personas/p.md not created")
	}
	if !env.TargetFileExists("delegates/d.md") {
		t.Fatal("delegates/d.md not created")
	}
	if !env.TargetFileExists("skills/s.md") {
		t.Fatal("skills/s.md not created")
	}

	if !containsWarning(result.Warnings, "persona assets") {
		t.Errorf("missing persona passive warning, warnings: %#v", result.Warnings)
	}
	if !containsWarning(result.Warnings, "delegate assets") {
		t.Errorf("missing delegate passive warning, warnings: %#v", result.Warnings)
	}

	persona := env.ReadTargetFile("personas/p.md")
	if !strings.Contains(persona, "name: Persona A") {
		t.Errorf("persona missing rendered name field: %q", persona)
	}

	delegate := env.ReadTargetFile("delegates/d.md")
	if !strings.Contains(delegate, "tools: Read, Edit") {
		t.Errorf("delegate missing tools frontmatter: %q", delegate)
	}

	skill := env.ReadTargetFile("skills/s.md")
	if !strings.Contains(skill, "name: skill-a") {
		t.Errorf("skill missing name frontmatter: %q", skill)
	}
	if strings.Contains(skill, "tools:") {
		t.Errorf("skill should not include tools frontmatter for codex: %q", skill)
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
