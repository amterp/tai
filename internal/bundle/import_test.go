package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/testutil"
)

// recordingLogger captures resolver messages so tests can assert on the
// warnings and info lines the resolver is expected to emit.
type recordingLogger struct {
	lines []string
}

func (l *recordingLogger) Printf(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *recordingLogger) contains(substr string) bool {
	for _, line := range l.lines {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

// producerRepo builds a config repo that offers bundles for import, and
// returns its root.
func producerRepo(t *testing.T, root string, files map[string]string) string {
	t.Helper()

	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// setupImportEnv builds a consumer repo plus a "work" producer repo whose
// bundles are importable, and returns both loaded sources.
//
// The producer deliberately holds a persona with the same filename as the
// consumer's, so a test can prove an imported bundle reaches its own repo
// rather than the consumer's.
func setupImportEnv(t *testing.T) (*testutil.TestEnv, map[string]*source.LoadedSource) {
	t.Helper()
	env := testutil.NewTestEnv(t)

	env.SetupAsset("personas", "senior-swe.md", "# Consumer senior-swe")
	env.SetupAsset("delegates", "code-reviewer.md", "# Consumer code-reviewer")

	workRoot := producerRepo(t, filepath.Join(env.Root, "work-repo"), map[string]string{
		"tai.yaml": `version: 1
sources:
  - name: recl
    path: ~/src/recl
`,
		"assets/personas/senior-swe.md": "# Work senior-swe",
		"assets/personas/work-style.md": "# Work style",
		"assets/skills/dredge/SKILL.md": "# Dredge",
		"assets/skills/beagle/SKILL.md": "# Beagle",
		"assets/delegates/auditor.md":   "# Auditor",
		"bundles/work-base.yaml": `name: work-base
assets:
  - type: persona
    path: assets/personas/work-style.md
`,
		"bundles/work-tools.yaml": `name: work-tools
bundles:
  - work-base
assets:
  - type: skill
    path: assets/skills/dredge
  - type: skill
    path: assets/skills/beagle
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

	return env, map[string]*source.LoadedSource{"local": local, "work": work}
}

// resolveOne resolves a single `bundles:` entry, the shape exclusion tests
// need since exclusions live on the entry rather than on the bundle.
func resolveOne(t *testing.T, r *Resolver, entry config.BundleEntry) (*ResolvedBundle, error) {
	t.Helper()
	resolved, _, err := r.ResolveMultiple([]config.BundleEntry{entry})
	return resolved, err
}

func assetNames(assets []*config.Asset) []string {
	names := make([]string, len(assets))
	for i, a := range assets {
		names[i] = a.Type + "/" + a.Name
	}
	return names
}

func hasAsset(assets []*config.Asset, typeAndName string) bool {
	for _, name := range assetNames(assets) {
		if name == typeAndName {
			return true
		}
	}
	return false
}

func TestResolveImportedBundle(t *testing.T) {
	_, sources := setupImportEnv(t)

	resolver := NewResolver(map[string]*config.Bundle{}, sources)
	resolved, err := resolver.Resolve("work:work-tools")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// work-tools includes work-base, so all three assets come through without
	// the consumer restating any of them.
	for _, want := range []string{"skill/dredge", "skill/beagle", "persona/work-style.md"} {
		if !hasAsset(resolved.Assets, want) {
			t.Errorf("missing %s; got %v", want, assetNames(resolved.Assets))
		}
	}

	for _, asset := range resolved.Assets {
		if asset.SourceName != "work" {
			t.Errorf("asset %s came from source %q, want 'work'", asset.Name, asset.SourceName)
		}
	}
}

// The origin rule: an unprefixed path inside an imported bundle means the
// repo that bundle came from. Getting this wrong would silently install the
// consumer's same-named asset instead of upstream's.
func TestImportedBundleResolvesAssetsAgainstItsOwnRepo(t *testing.T) {
	_, sources := setupImportEnv(t)

	work := sources["work"]
	bundles, err := work.LoadBundles()
	if err != nil {
		t.Fatal(err)
	}
	bundles["shadowed"] = &config.Bundle{
		Name: "shadowed",
		Assets: []config.AssetEntry{
			{Type: "persona", Path: "assets/personas/senior-swe.md"},
		},
	}

	resolver := NewResolver(map[string]*config.Bundle{}, sources)
	resolved, err := resolver.Resolve("work:shadowed")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if len(resolved.Assets) != 1 {
		t.Fatalf("len(Assets) = %d, want 1", len(resolved.Assets))
	}
	if got := resolved.Assets[0].SourceName; got != "work" {
		t.Errorf("resolved from source %q, want 'work' (the bundle's own repo)", got)
	}
	if content, err := os.ReadFile(resolved.Assets[0].SourcePath); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(string(content), "Work senior-swe") {
		t.Errorf("resolved the consumer's asset, not the origin repo's: %s", content)
	}
}

// A local bundle keeps resolving unprefixed paths against the active repo,
// which is what "origin" means for a bundle that was never imported.
func TestLocalBundleStillResolvesAgainstActiveRepo(t *testing.T) {
	_, sources := setupImportEnv(t)

	bundles := map[string]*config.Bundle{
		"core": {
			Name: "core",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"},
			},
		},
	}

	resolver := NewResolver(bundles, sources)
	resolved, err := resolver.Resolve("core")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got := resolved.Assets[0].SourceName; got != "local" {
		t.Errorf("resolved from source %q, want 'local'", got)
	}
}

func TestImportExclusionsReachThroughNestedIncludes(t *testing.T) {
	_, sources := setupImportEnv(t)

	logger := &recordingLogger{}
	resolver := NewResolver(map[string]*config.Bundle{}, sources)
	resolver.SetLogger(logger)

	resolved, err := resolveOne(t, resolver, config.BundleEntry{
		Name: "work:work-tools",
		// beagle is a direct asset of work-tools; work-style comes from the
		// nested work-base include. Both must be droppable from here.
		Exclude: []string{"assets/skills/beagle", "assets/personas/work-style.md"},
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if hasAsset(resolved.Assets, "skill/beagle") {
		t.Error("excluded direct asset still present")
	}
	if hasAsset(resolved.Assets, "persona/work-style.md") {
		t.Error("exclusion did not reach through the nested include")
	}
	if !hasAsset(resolved.Assets, "skill/dredge") {
		t.Errorf("exclusion took out more than it should; got %v", assetNames(resolved.Assets))
	}
}

// Upstream may delete an asset a consumer excluded, which satisfies the intent
// - so a stale pattern warns rather than failing someone else's install.
func TestUnmatchedExclusionWarnsButSucceeds(t *testing.T) {
	_, sources := setupImportEnv(t)

	logger := &recordingLogger{}
	resolver := NewResolver(map[string]*config.Bundle{}, sources)
	resolver.SetLogger(logger)

	resolved, err := resolveOne(t, resolver, config.BundleEntry{
		Name:    "work:work-tools",
		Exclude: []string{"assets/skills/no-such-thing"},
	})
	if err != nil {
		t.Fatalf("a stale exclusion must not fail resolution: %v", err)
	}
	if len(resolved.Assets) != 3 {
		t.Errorf("len(Assets) = %d, want 3", len(resolved.Assets))
	}
	if !logger.contains("matched no assets") {
		t.Errorf("expected a warning about the unmatched pattern; got %v", logger.lines)
	}
}

// A source-level exclusion is the consumer saying "never take this from that
// source". An imported bundle referencing it is upstream's reference, not the
// consumer's, so it is filtered rather than treated as a config contradiction.
func TestSourceExclusionFiltersImportedBundle(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupAsset("personas", "keep.md", "# Keep")

	workRoot := producerRepo(t, filepath.Join(env.Root, "work-repo"), map[string]string{
		"assets/skills/beagle/SKILL.md": "# Beagle",
		"assets/skills/dredge/SKILL.md": "# Dredge",
		"bundles/work-tools.yaml": `name: work-tools
assets:
  - type: skill
    path: assets/skills/dredge
  - type: skill
    path: assets/skills/beagle
`,
	})

	loader := source.NewLoader(env.ConfigRepo)
	local, err := loader.Load(config.Source{Name: "local", Type: "local", Path: "./assets"})
	if err != nil {
		t.Fatal(err)
	}
	work, err := loader.Load(config.Source{
		Name: "work", Type: "local", Path: workRoot,
		Exclude: []string{"assets/skills/beagle"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]*source.LoadedSource{"local": local, "work": work}

	logger := &recordingLogger{}
	resolver := NewResolver(map[string]*config.Bundle{
		// The same asset, named by the consumer's own config.
		"mine": {
			Name: "mine",
			Assets: []config.AssetEntry{
				{Type: "skill", Path: "work:assets/skills/beagle"},
			},
		},
	}, sources)
	resolver.SetLogger(logger)

	resolved, err := resolver.Resolve("work:work-tools")
	if err != nil {
		t.Fatalf("an imported bundle must not fail on the consumer's own exclusion: %v", err)
	}
	if hasAsset(resolved.Assets, "skill/beagle") {
		t.Error("excluded asset came through an imported bundle")
	}
	if !hasAsset(resolved.Assets, "skill/dredge") {
		t.Error("filtering took out an asset that was not excluded")
	}
	if !logger.contains("Excluded by source") {
		t.Errorf("the drop should be logged, not silent; got %v", logger.lines)
	}

	// The consumer naming it directly is still a contradiction in their own
	// config, and still errors.
	if _, err := resolver.Resolve("mine"); err == nil {
		t.Error("a locally written reference to an excluded asset must still error")
	}
}

func TestImportCycleAcrossSourcesDetected(t *testing.T) {
	env := testutil.NewTestEnv(t)

	workRoot := producerRepo(t, filepath.Join(env.Root, "work-repo"), map[string]string{
		"bundles/loop.yaml": `name: loop
bundles:
  - local:back
`,
	})

	loader := source.NewLoader(env.ConfigRepo)
	local, err := loader.Load(config.Source{Name: "local", Type: "local", Path: "."})
	if err != nil {
		t.Fatal(err)
	}
	work, err := loader.Load(config.Source{Name: "work", Type: "local", Path: workRoot})
	if err != nil {
		t.Fatal(err)
	}

	env.SetupBundle("back", `name: back
bundles:
  - work:loop
`)
	localBundles, err := local.LoadBundles()
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver(localBundles, map[string]*source.LoadedSource{"local": local, "work": work})
	_, err = resolver.Resolve("work:loop")
	if err == nil {
		t.Fatal("expected a circular reference error")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("error should name the cycle, got: %v", err)
	}
}

// An imported bundle can reach into a third source. The consumer has to
// declare it too, and the error should say what to add rather than only what
// is missing.
func TestImportedBundleNamingUndeclaredSourceExplainsItself(t *testing.T) {
	env, sources := setupImportEnv(t)
	_ = env

	work := sources["work"]
	bundles, err := work.LoadBundles()
	if err != nil {
		t.Fatal(err)
	}
	bundles["needs-third"] = &config.Bundle{
		Name: "needs-third",
		Assets: []config.AssetEntry{
			{Type: "skill", Path: "recl:assets/skills/recl"},
		},
	}

	resolver := NewResolver(map[string]*config.Bundle{}, sources)
	_, err = resolver.Resolve("work:needs-third")
	if err == nil {
		t.Fatal("expected an error for the undeclared source")
	}
	if !strings.Contains(err.Error(), "recl") {
		t.Errorf("error should name the missing source, got: %v", err)
	}
	// The origin repo's tai.yaml defines it, so the message can say where it
	// points instead of leaving the user to go find out.
	if !strings.Contains(err.Error(), "~/src/recl") {
		t.Errorf("error should quote how the origin repo defines it, got: %v", err)
	}
}

func TestImportedBundleNotFoundListsAvailable(t *testing.T) {
	_, sources := setupImportEnv(t)

	resolver := NewResolver(map[string]*config.Bundle{}, sources)
	_, err := resolver.Resolve("work:nope")
	if err == nil {
		t.Fatal("expected an error for the missing bundle")
	}
	if !strings.Contains(err.Error(), "work-tools") {
		t.Errorf("error should list what the source does offer, got: %v", err)
	}
}
