package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/adapter/codex"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/profile"
	"github.com/amterp/tai/internal/source"
)

// A skill pack written for Claude Code carries Claude's own frontmatter at the
// top level. A source declaring `native_frontmatter: claude` installs those
// fields for Claude as if written under native.claude - whether the skill is
// installed directly or its SKILL.md is included into another skill - and
// keeps them out of every other target, where they mean nothing.
func TestE2ESourceNativeFrontmatter(t *testing.T) {
	env := setupImportEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: work
    path: ` + env.producer + `
    native_frontmatter: claude
default_profile: dev
`)
	env.writeProducerFile(t, "assets/skills/dredge/SKILL.md", `---
name: dredge
description: The dredge skill
allowed-tools: [Read, Grep]
user-invocable: false
---

# dredge
`)
	env.WriteConfigRepoFile("assets/skills/mine/tai.yaml", `include:
  - from: work
    path: assets/skills/dredge/SKILL.md
    dest: SKILL.md
`)
	env.SetupBundle("mine", `name: mine
description: Local assets
assets:
  - type: skill
    path: skills/mine
`)
	env.SetupProfile("dev", `name: dev
description: Dev profile
target: claude
bundles:
  - work:work-tools
  - mine
`)

	installFor := func(adap adapter.Adapter) *install.InstallResult {
		t.Helper()
		taiConfig, err := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		sources, err := source.NewLoader(env.ConfigRepo).LoadAll(taiConfig.Sources)
		if err != nil {
			t.Fatal(err)
		}
		bundles, err := config.LoadBundlesFromDir(filepath.Join(env.ConfigRepo, "bundles"))
		if err != nil {
			t.Fatal(err)
		}
		profiles, err := config.LoadProfilesFromDir(filepath.Join(env.ConfigRepo, "profiles"))
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := profile.NewResolver(profiles, bundles, sources).Resolve("dev")
		if err != nil {
			t.Fatal(err)
		}
		installer := install.NewInstaller(install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, sources)
		result, err := installer.Install(adap, "dev", resolved.Assets, install.InstallOptions{})
		if err != nil {
			t.Fatalf("installing for %s: %v", adap.Name(), err)
		}
		return result
	}

	result := installFor(claude.New(env.TargetDir))
	for _, rel := range []string{"skills/dredge/SKILL.md", "skills/mine/SKILL.md"} {
		got := env.ReadTargetFile(rel)
		for _, want := range []string{"allowed-tools:", "- Read", "user-invocable: false"} {
			if !strings.Contains(got, want) {
				t.Errorf("claude %s missing %q:\n%s", rel, want, got)
			}
		}
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "allowed-tools") || strings.Contains(w, "user-invocable") {
			t.Errorf("claimed field still warned about: %s", w)
		}
	}

	codexDir := filepath.Join(env.Root, "codex-home")
	result = installFor(codex.New(codexDir))
	got := env.ReadFile(filepath.Join(codexDir, "skills", "dredge", "SKILL.md"))
	if strings.Contains(got, "allowed-tools") || strings.Contains(got, "user-invocable") {
		t.Errorf("Claude's fields reached the Codex copy:\n%s", got)
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "allowed-tools") {
			t.Errorf("codex warned about a field the source declared Claude's: %s", w)
		}
	}
}
