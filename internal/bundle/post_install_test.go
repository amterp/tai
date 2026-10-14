package bundle

import (
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/testutil"
)

// setupStepsEnv builds a "work" repo whose bundles declare post_install steps:
// tools-a and tools-b both include setup, so setup is reached twice.
func setupStepsEnv(t *testing.T) map[string]*source.LoadedSource {
	t.Helper()
	env := testutil.NewTestEnv(t)
	env.SetupAsset("personas", "mine.md", "# Mine")

	workRoot := producerRepo(t, filepath.Join(env.Root, "work-repo"), map[string]string{
		"assets/personas/work-style.md": "# Work style",
		"assets/skills/dredge/SKILL.md": "# Dredge",
		"assets/skills/beagle/SKILL.md": "# Beagle",
		"bundles/setup.yaml": `name: setup
assets:
  - type: persona
    path: assets/personas/work-style.md
post_install:
  - name: render
    command: [./scripts/render.sh]
`,
		"bundles/tools-a.yaml": `name: tools-a
bundles:
  - setup
assets:
  - type: skill
    path: assets/skills/dredge
post_install:
  - name: notify
    command: [./scripts/notify.sh]
`,
		"bundles/tools-b.yaml": `name: tools-b
bundles:
  - setup
assets:
  - type: skill
    path: assets/skills/beagle
post_install:
  - name: render
    command: [./scripts/other-render.sh]
`,
	})

	loader := source.NewLoader(env.ConfigRepo)
	local, err := loader.Load(config.Source{Name: "local", Type: "local", Path: "./assets"})
	if err != nil {
		t.Fatalf("loading local source: %v", err)
	}
	work, err := loader.Load(config.Source{Name: "work", Type: "local", Path: workRoot})
	if err != nil {
		t.Fatalf("loading work source: %v", err)
	}
	return map[string]*source.LoadedSource{"local": local, "work": work}
}

func stepKeys(steps []Step) []string {
	keys := make([]string, len(steps))
	for i, s := range steps {
		keys[i] = s.Bundle + "/" + s.Name
	}
	return keys
}

func TestBundleStepsOrderedAndDeduplicated(t *testing.T) {
	sources := setupStepsEnv(t)

	mine := &config.Bundle{
		Name:    "mine",
		Bundles: bundleRefs("work:tools-a", "work:tools-b"),
		Assets:  []config.AssetEntry{{Type: "persona", Path: "personas/mine.md"}},
		PostInstall: config.PostInstallSteps{
			{Name: "local-step", Command: []string{"./local.sh"}},
		},
		RootDir: "/config-repo",
	}
	resolver := NewResolver(map[string]*config.Bundle{"mine": mine}, sources)

	resolved, err := resolver.Resolve("mine")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// setup is reached through both tools bundles but runs once, before the
	// bundles including it; tools-b's own "render" is a different step.
	want := []string{"work:setup/render", "work:tools-a/notify", "work:tools-b/render", "mine/local-step"}
	got := stepKeys(resolved.PostInstall)
	if len(got) != len(want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("steps = %v, want %v", got, want)
		}
	}

	workRoot, err := filepath.Abs(sources["work"].ContentPath())
	if err != nil {
		t.Fatal(err)
	}
	if dir := resolved.PostInstall[0].RootDir; dir != workRoot {
		t.Errorf("imported step RootDir = %q, want the source root %q", dir, workRoot)
	}
	if dir := resolved.PostInstall[3].RootDir; dir != "/config-repo" {
		t.Errorf("local step RootDir = %q, want the bundle's own root", dir)
	}
}

func TestBundleStepsDeduplicatedAcrossProfileEntries(t *testing.T) {
	sources := setupStepsEnv(t)
	resolver := NewResolver(map[string]*config.Bundle{}, sources)

	resolved, _, err := resolver.ResolveMultiple(bundleRefs("work:tools-a", "work:setup"))
	if err != nil {
		t.Fatalf("ResolveMultiple failed: %v", err)
	}

	got := stepKeys(resolved.PostInstall)
	if len(got) != 2 || got[0] != "work:setup/render" || got[1] != "work:tools-a/notify" {
		t.Errorf("steps = %v, want [work:setup/render work:tools-a/notify]", got)
	}
}

// Exclusions carve assets out of an import; they say nothing about its steps.
func TestImportExclusionKeepsBundleSteps(t *testing.T) {
	sources := setupStepsEnv(t)
	resolver := NewResolver(map[string]*config.Bundle{}, sources)

	resolved, err := resolveOne(t, resolver, config.BundleEntry{
		Name:    "work:tools-a",
		Exclude: []string{"assets/skills/dredge"},
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got := stepKeys(resolved.PostInstall); len(got) != 2 {
		t.Errorf("steps = %v, want setup's and tools-a's", got)
	}
}
