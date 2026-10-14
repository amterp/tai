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

// A skip reaches the steps of bundles the import includes, and a plain name
// matches every step of that name: tools-b's render and setup's both go.
func TestSkipPostInstallReachesIncludedSteps(t *testing.T) {
	sources := setupStepsEnv(t)
	logger := &recordingLogger{}
	resolver := NewResolver(map[string]*config.Bundle{}, sources)
	resolver.SetLogger(logger)

	resolved, err := resolveOne(t, resolver, config.BundleEntry{
		Name:            "work:tools-b",
		SkipPostInstall: []string{"render"},
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got := stepKeys(resolved.PostInstall); len(got) != 0 {
		t.Errorf("steps = %v, want none", got)
	}
	if len(resolved.Assets) != 2 {
		t.Errorf("len(Assets) = %d, want 2: skipping steps must leave assets alone", len(resolved.Assets))
	}
	if !logger.contains(`Skipped: post_install step "render" from bundle "work:setup"`) {
		t.Errorf("expected a Skipped line naming the step's bundle; got %v", logger.lines)
	}
}

// A skip covers its own import only. Another entry reaching the same bundle
// without one still runs the step, as with exclusions.
func TestSkipPostInstallIsPerImport(t *testing.T) {
	sources := setupStepsEnv(t)
	resolver := NewResolver(map[string]*config.Bundle{}, sources)

	resolved, _, err := resolver.ResolveMultiple([]config.BundleEntry{
		{Name: "work:tools-a", SkipPostInstall: []string{"render"}},
		{Name: "work:setup"},
	})
	if err != nil {
		t.Fatalf("ResolveMultiple failed: %v", err)
	}
	got := stepKeys(resolved.PostInstall)
	if len(got) != 2 || got[0] != "work:tools-a/notify" || got[1] != "work:setup/render" {
		t.Errorf("steps = %v, want [work:tools-a/notify work:setup/render]", got)
	}
}

// A bundle's own `bundles:` entries can skip too, since nested entries
// resolve through the same path as a profile's.
func TestSkipPostInstallOnNestedEntry(t *testing.T) {
	sources := setupStepsEnv(t)
	mine := &config.Bundle{
		Name:    "mine",
		Bundles: []config.BundleEntry{{Name: "work:tools-a", SkipPostInstall: []string{"notify"}}},
	}
	resolver := NewResolver(map[string]*config.Bundle{"mine": mine}, sources)

	resolved, err := resolver.Resolve("mine")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got := stepKeys(resolved.PostInstall); len(got) != 1 || got[0] != "work:setup/render" {
		t.Errorf("steps = %v, want [work:setup/render]", got)
	}
}

// Upstream renaming or deleting a step already satisfies the skip, so a stale
// name warns rather than failing the install.
func TestUnmatchedSkipPostInstallWarnsButSucceeds(t *testing.T) {
	sources := setupStepsEnv(t)
	logger := &recordingLogger{}
	resolver := NewResolver(map[string]*config.Bundle{}, sources)
	resolver.SetLogger(logger)

	resolved, err := resolveOne(t, resolver, config.BundleEntry{
		Name:            "work:tools-a",
		SkipPostInstall: []string{"no-such-step"},
	})
	if err != nil {
		t.Fatalf("a stale skip must not fail resolution: %v", err)
	}
	if got := stepKeys(resolved.PostInstall); len(got) != 2 {
		t.Errorf("steps = %v, want setup's and tools-a's", got)
	}
	if !logger.contains(`skip_post_install name "no-such-step"`) {
		t.Errorf("expected a warning about the unmatched name; got %v", logger.lines)
	}
}
