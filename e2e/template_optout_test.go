package e2e

import (
	"path/filepath"
	"strings"
	"testing"
)

// Markdown written for a target rather than for tai routinely contains `{#`
// (heading anchors) and `{{`, which the template engine reads as syntax. A
// source declaring `template: false` installs as written, however its content
// arrives: as its own asset, through a skill include, or through include_asset.
func TestE2ESourceTemplateOptOut(t *testing.T) {
	env := setupImportEnv(t)

	const notTemplate = "## Remote transport {#remote-transport}\n\nUse {{ placeholder }} here.\n"

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: work
    path: ` + env.producer + `
    template: false
default_profile: dev
`)
	env.writeProducerFile(t, "assets/skills/dredge/SKILL.md", skill("dredge")+notTemplate)
	env.writeProducerFile(t, "assets/skills/dredge/ref.md", notTemplate)
	env.writeProducerFile(t, "assets/files/snippet.md", notTemplate)

	env.WriteConfigRepoFile("assets/skills/mine/SKILL.md", skill("mine")+"Target: {{ target_name }}\n")
	env.WriteConfigRepoFile("assets/skills/mine/tai.yaml", `include:
  - from: work
    path: assets/skills/dredge/ref.md
    dest: refs/
`)
	env.WriteConfigRepoFile("assets/personas/inliner.md", `---
tai-schema-version: 1
name: Inliner
description: Inlines a file from the work source
---

{% include_asset "work:assets/files/snippet.md" %}
`)
	env.SetupBundle("mine", `name: mine
description: Local assets
assets:
  - type: skill
    path: skills/mine
  - type: persona
    path: personas/inliner.md
`)
	env.SetupProfile("dev", `name: dev
description: Dev profile
target: claude
bundles:
  - work:work-tools
  - mine
`)

	env.installProfile(t, "dev")

	for _, rel := range []string{
		"skills/dredge/SKILL.md",
		"skills/dredge/ref.md",
		"skills/mine/refs/ref.md",
		"output-styles/inliner.md",
	} {
		if got := env.ReadTargetFile(rel); !strings.Contains(got, notTemplate) {
			t.Errorf("%s was templated; want the source text verbatim, got:\n%s", rel, got)
		}
	}

	// The opt-out belongs to the work source, so the local skill that includes
	// from it is still templated.
	if got := env.ReadTargetFile(filepath.Join("skills", "mine", "SKILL.md")); !strings.Contains(got, "Target: claude") {
		t.Errorf("skills/mine/SKILL.md was not templated:\n%s", got)
	}
}
