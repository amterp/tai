package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
	"github.com/amterp/tai/internal/template"
	"github.com/amterp/tai/internal/testutil"
)

// statusLineAsset builds an asset pointing at a statusline fixture.
func statusLineAsset(env *testutil.TestEnv, name string) *config.Asset {
	return &config.Asset{
		Type:       config.AssetTypeStatusLine,
		Name:       name,
		SourceName: "local",
		SourcePath: filepath.Join(env.ConfigRepo, "assets", "statuslines", name),
	}
}

// Config assets used to bypass the template engine entirely, so a {{ var }} in
// one was silently installed as a literal.
func TestConfigAssetIsTemplated(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	env.SetupAsset("statuslines", "custom.yaml", "level: custom\ncommand: {{ script_path }}\n")

	opts := InstallOptions{
		Variables: template.Variables{"script_path": "~/bin/statusline.sh"},
	}
	if _, err := installer.Install(adapter, "test-profile", []*config.Asset{statusLineAsset(env, "custom.yaml")}, opts); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	statusLine := settings["statusLine"].(map[string]any)
	if got := statusLine["command"]; got != "~/bin/statusline.sh" {
		t.Errorf("statusLine.command = %v, want the substituted value", got)
	}
}

func TestConfigAssetTemplateErrorIsReported(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)

	// An unclosed tag is a template error, not valid YAML content.
	env.SetupAsset("statuslines", "broken.yaml", "level: custom\ncommand: {% if %}\n")

	_, err := installer.Install(adapter, "test-profile", []*config.Asset{statusLineAsset(env, "broken.yaml")}, InstallOptions{})
	if err == nil {
		t.Fatal("Install succeeded, want a template error")
	}
	if !strings.Contains(err.Error(), "broken.yaml") {
		t.Errorf("error = %v, want it to name the offending asset", err)
	}
}

// Scope decides which of a target's config files an entry lands in. It used to
// be hardcoded to "user" in two places.
func TestConfigAssetScopeOverride(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)
	env.SetupAsset("statuslines", "bar.yaml", "level: minimal\n")

	// Project scope resolves against the working directory, so run there.
	wd := t.TempDir()
	restore := chdir(t, wd)
	defer restore()

	asset := statusLineAsset(env, "bar.yaml")
	asset.Scope = config.ScopeProject

	if _, err := installer.Install(adapter, "test-profile", []*config.Asset{asset}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(env.TargetDir, "settings.json")); !os.IsNotExist(err) {
		t.Error("user-scope settings.json was written despite a project-scope override")
	}
	if _, err := os.Stat(filepath.Join(wd, ".claude", "settings.json")); err != nil {
		t.Errorf("project-scope settings.json not written: %v", err)
	}
}

func TestConfigAssetProfileScopeDefault(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)
	env.SetupAsset("statuslines", "bar.yaml", "level: minimal\n")

	wd := t.TempDir()
	restore := chdir(t, wd)
	defer restore()

	opts := InstallOptions{Scope: config.ScopeLocal}
	if _, err := installer.Install(adapter, "test-profile", []*config.Asset{statusLineAsset(env, "bar.yaml")}, opts); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wd, ".claude", "settings.local.json")); err != nil {
		t.Errorf("profile scope 'local' not honored: %v", err)
	}
}

// An asset's own scope beats the profile default.
func TestConfigAssetScopePrecedence(t *testing.T) {
	ctx := &installContext{opts: InstallOptions{Scope: config.ScopeProject}}

	if got := ctx.scopeFor(&config.Asset{Scope: config.ScopeLocal}); got != config.ScopeLocal {
		t.Errorf("asset override = %q, want %q", got, config.ScopeLocal)
	}
	if got := ctx.scopeFor(&config.Asset{}); got != config.ScopeProject {
		t.Errorf("profile default = %q, want %q", got, config.ScopeProject)
	}

	bare := &installContext{opts: InstallOptions{}}
	if got := bare.scopeFor(&config.Asset{}); got != config.DefaultScope {
		t.Errorf("fallback = %q, want %q", got, config.DefaultScope)
	}
}

// Manifests written before edit tracking carry no Edits. Those entries were
// installed by replacing a whole settings key, so that is how they must come
// out - silently failing to remove them would strand config tai claims to own.
func TestLegacyConfigEntryWithoutEditsIsRemoved(t *testing.T) {
	env := testutil.NewTestEnv(t)
	manifestStore := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := NewInstaller(manifestStore, env.ConfigRepo, nil)
	adap := claude.New(env.TargetDir)

	settingsPath := filepath.Join(env.TargetDir, "settings.json")
	if err := os.MkdirAll(env.TargetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte(`{"statusLine":{"style":"minimal"},"keep":true}`), 0644); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	legacy := &Manifest{
		Version: ManifestVersion,
		Target:  adap.Name(),
		Profile: "test-profile",
		ConfigEntries: []ConfigEntry{{
			Type:         config.AssetTypeStatusLine,
			Name:         "old",
			SourceName:   "local",
			SettingsFile: settingsPath,
			Checksum:     "stale",
			// No Edits: this manifest predates edit tracking.
		}},
	}
	if err := manifestStore.Save(legacy); err != nil {
		t.Fatalf("saving legacy manifest: %v", err)
	}

	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	settings, err := readJSONFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	if _, exists := settings["statusLine"]; exists {
		t.Error("legacy statusLine entry was not removed")
	}
	if settings["keep"] != true {
		t.Error("unrelated settings key was removed")
	}
}

// Reinstalling an unchanged config asset must not accumulate entries or churn
// the file.
func TestConfigAssetReinstallIsIdempotent(t *testing.T) {
	env, installer, adapter := setupStatusLineTest(t)
	env.SetupAsset("statuslines", "bar.yaml", "level: detailed\nshow:\n  - cost\n")

	assets := []*config.Asset{statusLineAsset(env, "bar.yaml")}

	var last string
	for run := 1; run <= 3; run++ {
		result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
		if err != nil {
			t.Fatalf("install %d failed: %v", run, err)
		}
		if run > 1 && len(result.Unchanged) != 1 {
			t.Errorf("install %d: Unchanged = %v, want one entry", run, result.Unchanged)
		}

		data, err := os.ReadFile(filepath.Join(env.TargetDir, "settings.json"))
		if err != nil {
			t.Fatalf("reading settings.json: %v", err)
		}
		if run > 1 && string(data) != last {
			t.Errorf("install %d changed settings.json:\n got: %s\nwant: %s", run, data, last)
		}
		last = string(data)
	}
}

func TestRegistryKnowsStatusLine(t *testing.T) {
	h, ok := configasset.Lookup(config.AssetTypeStatusLine)
	if !ok {
		t.Fatal("no handler registered for statusline")
	}
	if h.Cardinality() != configasset.Singleton {
		t.Error("statusline should be a singleton - a target shows one status line")
	}
	if configasset.IsCollection(config.AssetTypeStatusLine) {
		t.Error("IsCollection(statusline) = true, want false")
	}
}

// chdir switches to dir for the duration of a test, returning a restore func.
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restoring cwd: %v", err)
		}
	}
}
