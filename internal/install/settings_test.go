package install

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/codex"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func settingsAsset(env *testutil.TestEnv, name string) *config.Asset {
	return &config.Asset{
		Type:       config.AssetTypeSettings,
		Name:       name,
		SourceName: "local",
		SourcePath: filepath.Join(env.ConfigRepo, "assets", "settings", name),
	}
}

// seedSettings writes a settings.json holding state tai did not write.
func seedSettings(t *testing.T, targetDir, content string) string {
	t.Helper()
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(targetDir, "settings.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return path
}

func mustReadSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	settings, err := readJSONFile(path)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	return settings
}

func TestSettingsMergeBesideForeignState(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	path := seedSettings(t, env.TargetDir, `{
  "model": "opus",
  "env": {"USER_VAR": "1"},
  "permissions": {"allow": ["Read"]}
}`)

	env.SetupAsset("settings", "afk.yaml", `native:
  claude:
    env:
      CLAUDE_AFK_TIMEOUT_MS: "600000"
    enabledPlugins:
      "foo@bar": true
    defaultMode: auto
    permissions:
      allow: ["Bash(ls:*)"]
`)
	assets := []*config.Asset{settingsAsset(env, "afk.yaml")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings := mustReadSettings(t, path)
	envBlock := settings["env"].(map[string]any)
	if envBlock["USER_VAR"] != "1" || envBlock["CLAUDE_AFK_TIMEOUT_MS"] != "600000" {
		t.Errorf("env = %v, want the user's var beside tai's", envBlock)
	}
	if settings["model"] != "opus" || settings["defaultMode"] != "auto" {
		t.Errorf("model/defaultMode = %v/%v", settings["model"], settings["defaultMode"])
	}
	if plugins := settings["enabledPlugins"].(map[string]any); plugins["foo@bar"] != true {
		t.Errorf("enabledPlugins = %v", plugins)
	}
	allow := settings["permissions"].(map[string]any)["allow"].([]any)
	if !slices.Equal(allow, []any{"Read", "Bash(ls:*)"}) {
		t.Errorf("permissions.allow = %v, want the user's entry then tai's", allow)
	}

	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	if len(result.Unchanged) != 1 {
		t.Errorf("reinstall: Unchanged = %v, want the asset", result.Unchanged)
	}

	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	settings = mustReadSettings(t, path)
	if _, ok := settings["defaultMode"]; ok {
		t.Error("defaultMode survived uninstall")
	}
	if _, ok := settings["enabledPlugins"]; ok {
		t.Error("enabledPlugins should be pruned once tai's only entry is gone")
	}
	if env := settings["env"].(map[string]any); len(env) != 1 || env["USER_VAR"] != "1" {
		t.Errorf("env after uninstall = %v, want only the user's var", env)
	}
	if allow := settings["permissions"].(map[string]any)["allow"].([]any); !slices.Equal(allow, []any{"Read"}) {
		t.Errorf("permissions.allow after uninstall = %v", allow)
	}
}

func TestSettingsEditedLeafConflictsAndSurvivesRemoval(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "afk.yaml", "native: {claude: {env: {AFK: \"1\", OTHER: \"x\"}}}\n")
	assets := []*config.Asset{settingsAsset(env, "afk.yaml")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	path := filepath.Join(env.TargetDir, "settings.json")
	settings := mustReadSettings(t, path)
	settings["env"].(map[string]any)["AFK"] = "edited"
	writeJSONFile(t, path, settings)

	_, err := installer.Install(adap, "p", assets, InstallOptions{})
	var conflicts *ConflictsError
	if !errors.As(err, &conflicts) {
		t.Fatalf("reinstall over an edited var: err = %v, want a conflict", err)
	}

	if _, err := installer.Install(adap, "p", nil, InstallOptions{}); err != nil {
		t.Fatalf("Install without the asset failed: %v", err)
	}
	env2 := mustReadSettings(t, path)["env"].(map[string]any)
	if env2["AFK"] != "edited" {
		t.Errorf("the user's edit was removed: env = %v", env2)
	}
	if _, ok := env2["OTHER"]; ok {
		t.Errorf("tai's untouched var should be removed: env = %v", env2)
	}
}

func TestSettingsAddedFieldConflictsWithUsersValue(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	seedSettings(t, env.TargetDir, `{"defaultMode": "plan"}`)

	env.SetupAsset("settings", "s.yaml", "native: {claude: {env: {A: \"1\"}}}\n")
	assets := []*config.Asset{settingsAsset(env, "s.yaml")}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// The asset gains a field the user already set by hand. Being managed
	// already must not wave the new field through.
	env.SetupAsset("settings", "s.yaml", "native: {claude: {env: {A: \"1\"}, defaultMode: auto}}\n")
	_, err := installer.Install(adap, "p", assets, InstallOptions{})
	var conflicts *ConflictsError
	if !errors.As(err, &conflicts) {
		t.Fatalf("err = %v, want a conflict on defaultMode", err)
	}
	if !strings.Contains(conflicts.Conflicts[0].Reason, "defaultMode") {
		t.Errorf("conflict reason = %q, want it to name defaultMode", conflicts.Conflicts[0].Reason)
	}
}

func TestSettingsAssetsShareAnObject(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "a.yaml", "native: {claude: {env: {A: \"1\"}, permissions: {allow: [x]}}}\n")
	env.SetupAsset("settings", "b.yaml", "native: {claude: {env: {B: \"2\"}, permissions: {allow: [y]}}}\n")
	assets := []*config.Asset{settingsAsset(env, "a.yaml"), settingsAsset(env, "b.yaml")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}

	path := filepath.Join(env.TargetDir, "settings.json")
	settings := mustReadSettings(t, path)
	if env := settings["env"].(map[string]any); env["A"] != "1" || env["B"] != "2" {
		t.Errorf("env = %v, want both assets' vars", env)
	}
	if allow := settings["permissions"].(map[string]any)["allow"].([]any); len(allow) != 2 {
		t.Errorf("permissions.allow = %v, want both assets' elements", allow)
	}

	if _, err := installer.Install(adap, "p", assets[1:], InstallOptions{}); err != nil {
		t.Fatalf("Install without a failed: %v", err)
	}
	settings = mustReadSettings(t, path)
	if env := settings["env"].(map[string]any); len(env) != 1 || env["B"] != "2" {
		t.Errorf("env = %v, want only b's var", env)
	}
}

func TestSettingsSameFieldInTwoAssetsIsAnError(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "a.yaml", "native: {claude: {env: {A: \"1\"}}}\n")
	env.SetupAsset("settings", "b.yaml", "native: {claude: {env: {A: \"2\"}}}\n")
	assets := []*config.Asset{settingsAsset(env, "a.yaml"), settingsAsset(env, "b.yaml")}

	_, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "env.A") {
		t.Fatalf("err = %v, want one naming env.A", err)
	}
	if _, statErr := os.Stat(filepath.Join(env.TargetDir, "settings.json")); !os.IsNotExist(statErr) {
		t.Error("settings.json was written despite the error")
	}
}

// A field moving from one asset to another in a single install is tai's own
// entry changing hands, not a conflict, and the old owner's removal must not
// take out the value the new owner just wrote.
func TestSettingsFieldMovesBetweenAssets(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	// From b to a, so the new owner installs before the old owner's removal
	// runs: the order in which the removal could take out the new write.
	env.SetupAsset("settings", "a.yaml", "native: {claude: {env: {B: \"2\"}}}\n")
	env.SetupAsset("settings", "b.yaml", "native: {claude: {env: {A: \"1\", KEEP: k}}}\n")
	assets := []*config.Asset{settingsAsset(env, "a.yaml"), settingsAsset(env, "b.yaml")}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	env.SetupAsset("settings", "a.yaml", "native: {claude: {env: {A: \"1\", B: \"2\"}}}\n")
	env.SetupAsset("settings", "b.yaml", "native: {claude: {env: {KEEP: k}}}\n")
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install after the move failed: %v", err)
	}

	path := filepath.Join(env.TargetDir, "settings.json")
	if env := mustReadSettings(t, path)["env"].(map[string]any); env["A"] != "1" {
		t.Errorf("env = %v, want A kept by its new owner", env)
	}

	// The new owner now holds it: dropping a takes A with it.
	if _, err := installer.Install(adap, "p", assets[1:], InstallOptions{}); err != nil {
		t.Fatalf("Install without a failed: %v", err)
	}
	if env := mustReadSettings(t, path)["env"].(map[string]any); len(env) != 1 || env["KEEP"] != "k" {
		t.Errorf("env = %v, want only b's KEEP", env)
	}
}

func TestSettingsRejectsKeysOtherTypesOwn(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "s.yaml", "native: {claude: {hooks: {Stop: []}}}\n")

	_, err := installer.Install(adap, "p", []*config.Asset{settingsAsset(env, "s.yaml")}, InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "hook assets") {
		t.Fatalf("err = %v, want one pointing at hook assets", err)
	}
}

func TestSettingsUnsupportedTargetWarnsOnlyWhenNamed(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	adap := codex.New(env.TargetDir)

	env.SetupAsset("settings", "claude-only.yaml", "native: {claude: {env: {A: \"1\"}}}\n")
	env.SetupAsset("settings", "both.yaml", "native: {claude: {env: {A: \"1\"}}, codex: {model: x}}\n")

	result, err := installer.Install(adap, "p", []*config.Asset{
		settingsAsset(env, "claude-only.yaml"),
		settingsAsset(env, "both.yaml"),
	}, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	joined := strings.Join(result.Warnings, "\n")
	if strings.Contains(joined, "claude-only") {
		t.Errorf("warned about an asset with nothing for codex: %v", result.Warnings)
	}
	if !strings.Contains(joined, "both.yaml") {
		t.Errorf("no warning for an asset with a codex block: %v", result.Warnings)
	}
}
