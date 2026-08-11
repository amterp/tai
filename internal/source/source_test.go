package source

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func TestLoaderLoad(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Set up assets
	env.SetupAsset("personas", "senior-swe.md", "# Senior SWE Persona")
	env.SetupAsset("personas", "brainstormer.md", "# Brainstormer Persona")
	env.SetupAsset("delegates", "code-reviewer.md", "# Code Reviewer")
	env.SetupAsset("files", "README.md", "# Files")

	// Create a skill directory
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)

	src := config.Source{
		Name: "local",
		Type: "local",
		Path: "./assets",
	}

	ls, err := loader.Load(src)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check resolved path
	expectedPath := filepath.Join(env.ConfigRepo, "assets")
	if ls.ResolvedPath != expectedPath {
		t.Errorf("ResolvedPath = %q, want %q", ls.ResolvedPath, expectedPath)
	}

	// Check discovered assets
	assertDiscovers(t, ls,
		"personas/senior-swe.md",
		"personas/brainstormer.md",
		"delegates/code-reviewer.md",
		"skills/my-skill",
		"files/README.md",
	)
}

// assertDiscovers checks that a source discovered exactly the given
// root-relative asset paths.
func assertDiscovers(t *testing.T, ls *LoadedSource, want ...string) {
	t.Helper()

	got := ls.ListAllAssets()
	found := make(map[string]bool, len(got))
	for _, path := range got {
		found[path] = true
	}

	for _, path := range want {
		if !found[path] {
			t.Errorf("asset %q not discovered; got %v", path, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("discovered %d assets, want %d; got %v", len(got), len(want), got)
	}
}

// A source pointed at a config repo root keeps its assets under assets/. The
// path is taken literally - nothing is appended - so the reference carries the
// assets/ prefix and discovery reports it that way too.
func TestLoaderLoadRepoRoot(t *testing.T) {
	env := testutil.NewTestEnv(t)

	repoPath := filepath.Join(env.Root, "external-tai-repo")
	assetsPath := filepath.Join(repoPath, "assets", "personas")
	if err := os.MkdirAll(assetsPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsPath, "external.md"), []byte("# External"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)
	ls, err := loader.Load(config.Source{Name: "external", Type: "local", Path: repoPath})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if ls.ResolvedPath != repoPath {
		t.Errorf("ResolvedPath = %q, want %q (the path, untouched)", ls.ResolvedPath, repoPath)
	}

	assertDiscovers(t, ls, "assets/personas/external.md")

	if _, ok := ls.GetAsset("assets/personas/external.md"); !ok {
		t.Error("GetAsset should resolve the path discovery reported")
	}
	if _, ok := ls.GetAsset("personas/external.md"); ok {
		t.Error("GetAsset must not silently strip or add an assets/ prefix")
	}
}

// Both layouts enumerate: a repo keeps assets under assets/, a plain asset
// folder has the type dirs at its top level, and one source can hold both
// without either shadowing the other.
func TestLoaderDiscoversBothLayouts(t *testing.T) {
	env := testutil.NewTestEnv(t)

	root := filepath.Join(env.Root, "mixed")
	for _, dir := range []string{
		filepath.Join(root, "personas"),
		filepath.Join(root, "assets", "personas"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "p.md"), []byte("# P"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	loader := NewLoader(env.ConfigRepo)
	ls, err := loader.Load(config.Source{Name: "mixed", Type: "local", Path: root})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	assertDiscovers(t, ls, "personas/p.md", "assets/personas/p.md")
}

func TestLoaderLoadAll(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupAsset("personas", "local-persona.md", "# Local")

	// Create secondary source
	secondaryPath := filepath.Join(env.Root, "secondary")
	secondaryAssets := filepath.Join(secondaryPath, "personas")
	if err := os.MkdirAll(secondaryAssets, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondaryAssets, "secondary-persona.md"), []byte("# Secondary"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)

	sources := []config.Source{
		{Name: "local", Type: "local", Path: "./assets"},
		{Name: "secondary", Type: "local", Path: secondaryPath},
	}

	loaded, err := loader.LoadAll(sources)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("len(loaded) = %d, want 2", len(loaded))
	}
	if _, ok := loaded["local"]; !ok {
		t.Error("missing 'local' source")
	}
	if _, ok := loaded["secondary"]; !ok {
		t.Error("missing 'secondary' source")
	}
}

func TestLoaderLoadMissing(t *testing.T) {
	env := testutil.NewTestEnv(t)
	loader := NewLoader(env.ConfigRepo)

	src := config.Source{
		Name: "missing",
		Type: "local",
		Path: "./nonexistent",
	}

	_, err := loader.Load(src)
	if err == nil {
		t.Error("expected error for missing source, got nil")
	}
}

func TestLoadedSourceGetAsset(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupAsset("personas", "test.md", "# Test")

	loader := NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	// Existing asset
	path, ok := ls.GetAsset("personas/test.md")
	if !ok {
		t.Error("GetAsset returned false for existing asset")
	}
	expectedPath := filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")
	if path != expectedPath {
		t.Errorf("path = %q, want %q", path, expectedPath)
	}

	// Non-existing asset
	_, ok = ls.GetAsset("personas/nonexistent.md")
	if ok {
		t.Error("GetAsset returned true for non-existing asset")
	}
}

func TestLoadedSourceResolveAsset(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupAsset("personas", "test.md", "# Test")

	// Create a skill directory
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve file asset
	asset, err := ls.ResolveAsset("personas/test.md", config.AssetTypePersona)
	if err != nil {
		t.Fatalf("ResolveAsset failed: %v", err)
	}
	if asset.Type != "persona" {
		t.Errorf("Type = %q, want 'persona'", asset.Type)
	}
	if asset.Name != "test.md" {
		t.Errorf("Name = %q, want 'test.md'", asset.Name)
	}
	if asset.SourceName != "local" {
		t.Errorf("SourceName = %q, want 'local'", asset.SourceName)
	}
	if asset.IsDir {
		t.Error("IsDir = true, want false")
	}

	// Resolve skill (directory) asset
	skillAsset, err := ls.ResolveAsset("skills/my-skill", config.AssetTypeSkill)
	if err != nil {
		t.Fatalf("ResolveAsset failed for skill: %v", err)
	}
	if !skillAsset.IsDir {
		t.Error("IsDir = false for skill, want true")
	}

	// Non-existing asset
	_, err = ls.ResolveAsset("personas/nonexistent.md", config.AssetTypePersona)
	if err == nil {
		t.Error("expected error for non-existing asset")
	}

	// Invalid asset type - WriteFile fails because the "invalid" directory doesn't exist,
	// confirming invalid asset type directories aren't created by default.
	err = os.WriteFile(filepath.Join(env.ConfigRepo, "assets", "invalid", "test.md"), []byte("x"), 0644)
	if err == nil {
		t.Error("expected error writing to non-existent 'invalid' asset type directory")
	}
}

func TestLoadedSourceListAllAssets(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupAsset("personas", "one.md", "# One")
	env.SetupAsset("personas", "two.md", "# Two")
	env.SetupAsset("delegates", "helper.md", "# Helper")

	loader := NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	all := ls.ListAllAssets()
	if len(all) != 3 {
		t.Errorf("len(all) = %d, want 3", len(all))
	}

	// Check that paths are correctly formatted
	found := make(map[string]bool)
	for _, path := range all {
		found[path] = true
	}
	for _, expected := range []string{"personas/one.md", "personas/two.md", "delegates/helper.md"} {
		if !found[expected] {
			t.Errorf("missing expected asset: %s", expected)
		}
	}
}

func TestLoaderCommandSource(t *testing.T) {
	commandSource := func(argv ...string) config.Source {
		return config.Source{
			Name:    "private",
			Type:    config.SourceTypeCommand,
			Command: argv,
		}
	}

	t.Run("resolvePath points into cache dir", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		path, err := loader.resolvePath(commandSource("sh", "-c", "true"))
		if err != nil {
			t.Fatalf("resolvePath() error = %v", err)
		}
		wantPrefix := filepath.Join(env.CacheDir, "sources") + string(filepath.Separator)
		if !strings.HasPrefix(path, wantPrefix) {
			t.Errorf("resolved path %q not under %q", path, wantPrefix)
		}
		if filepath.Base(path) != "private" {
			t.Errorf("resolved path %q should end in source name", path)
		}
	})

	t.Run("load without cache dir errors", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)

		if _, err := loader.Load(commandSource("sh", "-c", "true")); err == nil {
			t.Fatal("expected error when no cache dir configured")
		}
	})

	t.Run("load with missing cache tolerated", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		ls, err := loader.Load(commandSource("sh", "-c", "true"))
		if err != nil {
			t.Fatalf("Load() error = %v (missing cache must not fail the load)", err)
		}
		if ls.Materialized {
			t.Error("Materialized should be false before first fetch")
		}
		if err := ls.RequireMaterialized(); err == nil {
			t.Error("RequireMaterialized() should error before first fetch")
		} else if !strings.Contains(err.Error(), "tai refresh") {
			t.Errorf("error should hint at 'tai refresh', got: %v", err)
		}
		if got := len(ls.ListAllAssets()); got != 0 {
			t.Errorf("unfetched source should list 0 assets, got %d", got)
		}
	})

	t.Run("refresh materializes then load discovers assets", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("command source tests require POSIX sh")
		}
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		src := commandSource("sh", "-c", `
			mkdir -p "$TAI_DEST_DIR/personas" "$TAI_DEST_DIR/skills/tool"
			printf '# P' > "$TAI_DEST_DIR/personas/p.md"
			printf '# S' > "$TAI_DEST_DIR/skills/tool/SKILL.md"
			printf '%s' "$TAI_SOURCE" > "$TAI_DEST_DIR/personas/source-name.md"
		`)

		if err := loader.Refresh(src, "1.2.3"); err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}

		ls, err := loader.Load(src)
		if err != nil {
			t.Fatalf("Load() after refresh error = %v", err)
		}
		if !ls.Materialized {
			t.Error("Materialized should be true after refresh")
		}
		if err := ls.RequireMaterialized(); err != nil {
			t.Errorf("RequireMaterialized() after refresh = %v", err)
		}

		wantAssets := map[string]bool{
			filepath.Join("personas", "p.md"):           false,
			filepath.Join("personas", "source-name.md"): false,
			filepath.Join("skills", "tool"):             false,
		}
		for _, a := range ls.ListAllAssets() {
			if _, ok := wantAssets[a]; ok {
				wantAssets[a] = true
			}
		}
		for a, found := range wantAssets {
			if !found {
				t.Errorf("asset %q not discovered; got %v", a, ls.ListAllAssets())
			}
		}

		got, err := os.ReadFile(filepath.Join(ls.ResolvedPath, "personas", "source-name.md"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "private" {
			t.Errorf("TAI_SOURCE = %q, want 'private'", string(got))
		}
	})

	t.Run("failed refresh preserves previous cache", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("command source tests require POSIX sh")
		}
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		good := commandSource("sh", "-c", `mkdir -p "$TAI_DEST_DIR/x"; printf 'v1' > "$TAI_DEST_DIR/x/f"`)
		if err := loader.Refresh(good, ""); err != nil {
			t.Fatalf("initial Refresh() error = %v", err)
		}

		bad := commandSource("sh", "-c", "exit 1")
		if err := loader.Refresh(bad, ""); err == nil {
			t.Fatal("expected error from failing fetch")
		}

		ls, err := loader.Load(good)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(ls.ResolvedPath, "x", "f"))
		if err != nil || string(got) != "v1" {
			t.Errorf("previous cache should survive failed refresh, got %q, err %v", string(got), err)
		}
	})

	t.Run("refresh on non-fetchable source rejected", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		err := loader.Refresh(config.Source{Name: "local", Type: "local", Path: "./assets"}, "")
		if err == nil {
			t.Fatal("expected error refreshing a source that is not fetched")
		}
		if !strings.Contains(err.Error(), "cannot be fetched") {
			t.Errorf("error = %v, want it to say the source cannot be fetched", err)
		}
	})
}

func TestLoaderURLSource(t *testing.T) {
	urlSource := func(rawURL string) config.Source {
		return config.Source{Name: "upstream", Type: config.SourceTypeURL, URL: rawURL}
	}

	t.Run("resolvePath points into cache dir", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		path, err := loader.resolvePath(urlSource("https://example.com/GUIDE.md"))
		if err != nil {
			t.Fatalf("resolvePath() error = %v", err)
		}
		wantPrefix := filepath.Join(env.CacheDir, "sources") + string(filepath.Separator)
		if !strings.HasPrefix(path, wantPrefix) {
			t.Errorf("resolved path %q not under %q", path, wantPrefix)
		}
		if filepath.Base(path) != "upstream" {
			t.Errorf("resolved path %q should end in source name", path)
		}
	})

	t.Run("load with missing cache tolerated", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		ls, err := loader.Load(urlSource("https://example.com/GUIDE.md"))
		if err != nil {
			t.Fatalf("Load() error = %v (missing cache must not fail the load)", err)
		}
		if ls.Materialized {
			t.Error("Materialized should be false before first fetch")
		}
		if err := ls.RequireMaterialized(); err == nil {
			t.Error("RequireMaterialized() should error before first fetch")
		} else if !strings.Contains(err.Error(), "tai refresh") {
			t.Errorf("error should hint at 'tai refresh', got: %v", err)
		}
	})

	t.Run("refresh writes the file into a one-file directory", func(t *testing.T) {
		var gotUA string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			_, _ = w.Write([]byte("# Guide\n"))
		}))
		defer server.Close()

		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		src := urlSource(server.URL + "/GUIDE.md")
		if err := loader.Refresh(src, "1.2.3"); err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}

		ls, err := loader.Load(src)
		if err != nil {
			t.Fatalf("Load() after refresh error = %v", err)
		}
		if !ls.Materialized {
			t.Error("Materialized should be true after refresh")
		}

		got, err := os.ReadFile(filepath.Join(ls.ResolvedPath, "GUIDE.md"))
		if err != nil {
			t.Fatalf("reading fetched file: %v", err)
		}
		if string(got) != "# Guide\n" {
			t.Errorf("content = %q, want %q", got, "# Guide\n")
		}
		if gotUA != "tai/1.2.3" {
			t.Errorf("User-Agent = %q, want %q", gotUA, "tai/1.2.3")
		}
	})

	t.Run("url source carries no assets", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("# Guide\n"))
		}))
		defer server.Close()

		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		src := urlSource(server.URL + "/GUIDE.md")
		if err := loader.Refresh(src, ""); err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		ls, err := loader.Load(src)
		if err != nil {
			t.Fatalf("Load() error = %v (a source with no asset dirs must still load)", err)
		}
		if got := len(ls.ListAllAssets()); got != 0 {
			t.Errorf("url source should carry 0 assets, got %d", got)
		}
	})

	t.Run("explicit filename is honored", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("x"))
		}))
		defer server.Close()

		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		src := urlSource(server.URL + "/raw")
		src.Filename = "GUIDE.md"
		if err := loader.Refresh(src, ""); err != nil {
			t.Fatalf("Refresh() error = %v", err)
		}
		ls, err := loader.Load(src)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(ls.ResolvedPath, "GUIDE.md")); err != nil {
			t.Errorf("expected the file to be stored as GUIDE.md: %v", err)
		}
	})

	t.Run("failed refresh preserves previous cache", func(t *testing.T) {
		serve := "v1"
		status := http.StatusOK
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(serve))
		}))
		defer server.Close()

		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		src := urlSource(server.URL + "/GUIDE.md")
		if err := loader.Refresh(src, ""); err != nil {
			t.Fatalf("initial Refresh() error = %v", err)
		}

		status = http.StatusInternalServerError
		if err := loader.Refresh(src, ""); err == nil {
			t.Fatal("expected error from failing fetch")
		}

		ls, err := loader.Load(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(ls.ResolvedPath, "GUIDE.md"))
		if err != nil || string(got) != "v1" {
			t.Errorf("previous cache should survive failed refresh, got %q, err %v", string(got), err)
		}
	})
}

func TestLoadBundles(t *testing.T) {
	t.Run("reads bundles from the source root", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		repo := filepath.Join(env.Root, "producer")
		if err := os.MkdirAll(filepath.Join(repo, "bundles"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "bundles", "tools.yaml"),
			[]byte("name: work-tools\ndescription: Work tooling\n"), 0644); err != nil {
			t.Fatal(err)
		}

		ls, err := NewLoader(env.ConfigRepo).Load(config.Source{Name: "work", Type: "local", Path: repo})
		if err != nil {
			t.Fatal(err)
		}

		bundles, err := ls.LoadBundles()
		if err != nil {
			t.Fatalf("LoadBundles() error = %v", err)
		}
		// Keyed by the bundle's `name:` field, not its filename.
		if _, ok := bundles["work-tools"]; !ok {
			t.Errorf("bundle not found by name; got %v", bundles)
		}
	})

	t.Run("no bundles dir is not an error", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		ls, err := NewLoader(env.ConfigRepo).Load(config.Source{Name: "local", Type: "local", Path: "./assets"})
		if err != nil {
			t.Fatal(err)
		}

		bundles, err := ls.LoadBundles()
		if err != nil {
			t.Fatalf("offering no bundles must not be an error: %v", err)
		}
		if len(bundles) != 0 {
			t.Errorf("len(bundles) = %d, want 0", len(bundles))
		}
	})

	t.Run("unfetched command source reports the missing fetch", func(t *testing.T) {
		env := testutil.NewTestEnv(t)
		loader := NewLoader(env.ConfigRepo)
		loader.SetCacheDir(env.CacheDir)

		ls, err := loader.Load(config.Source{
			Name: "remote", Type: config.SourceTypeCommand, Command: []string{"sh", "-c", "true"},
		})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := ls.LoadBundles(); err == nil {
			t.Fatal("expected an error before the first fetch")
		} else if !strings.Contains(err.Error(), "tai refresh") {
			t.Errorf("error should hint at 'tai refresh', got: %v", err)
		}
	})
}

func TestLoaderExcludeHidesAssets(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupAsset("personas", "keeper.md", "# Keeper")
	env.SetupAsset("personas", "draft.md", "# Draft")

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "legacy-thing")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Legacy"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)
	ls, err := loader.Load(config.Source{
		Name: "local", Type: "local", Path: "./assets",
		Exclude: []string{"personas/draft.md", "skills/legacy-*"},
	})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Excluded assets must not appear in discovery.
	assertDiscovers(t, ls, "personas/keeper.md")

	// ...nor be reachable by naming them directly. They are still on disk, so
	// this only holds because GetAsset checks exclusions explicitly.
	if _, ok := ls.GetAsset("personas/draft.md"); ok {
		t.Error("GetAsset returned an excluded asset")
	}
	if _, ok := ls.GetAsset("skills/legacy-thing"); ok {
		t.Error("GetAsset returned an asset under an excluded directory")
	}
	if _, ok := ls.GetAsset("personas/keeper.md"); !ok {
		t.Error("GetAsset must still return non-excluded assets")
	}

	// The error has to point at the exclude list, not at the source.
	_, err = ls.ResolveAsset("personas/draft.md", "persona")
	if err == nil {
		t.Fatal("expected an error resolving an excluded asset")
	}
	if !strings.Contains(err.Error(), "excluded by source") {
		t.Errorf("error = %q, want it to name the exclusion", err)
	}
}
