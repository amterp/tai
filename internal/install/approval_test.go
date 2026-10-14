package install

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/codex"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

const personalPolicy = `# Shared policy

Notes for whoever edits this file - not installed.

## context: Push policy

An ordinary push to the working repo's existing
remote is consistent with policy.

Force pushes still warrant scrutiny.

## allow claude: Directing agents in Muster

Briefing agents is authorized.

## deny codex: Codex only

Not for Claude.
`

func approvalAsset(env *testutil.TestEnv, name string) *config.Asset {
	return &config.Asset{
		Type:       config.AssetTypeApproval,
		Name:       name,
		SourceName: "local",
		SourcePath: filepath.Join(env.ConfigRepo, "assets", "approvals", name),
	}
}

func autoModeArray(t *testing.T, settings map[string]any, key string) []any {
	t.Helper()
	autoMode, ok := settings["autoMode"].(map[string]any)
	if !ok {
		return nil
	}
	arr, _ := autoMode[key].([]any)
	return arr
}

func TestApprovalSeedsArraysItCreates(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("approvals", "personal.md", personalPolicy)
	assets := []*config.Asset{approvalAsset(env, "personal.md")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	path := filepath.Join(env.TargetDir, "settings.json")
	settings := mustReadSettings(t, path)
	wantEnv := []any{"$defaults",
		"Push policy: An ordinary push to the working repo's existing remote is consistent with policy.\n\n" +
			"Force pushes still warrant scrutiny."}
	if got := autoModeArray(t, settings, "environment"); !slices.Equal(got, wantEnv) {
		t.Errorf("environment = %q, want %q", got, wantEnv)
	}
	if got := autoModeArray(t, settings, "allow"); !slices.Equal(got, []any{"$defaults", "Directing agents in Muster: Briefing agents is authorized."}) {
		t.Errorf("allow = %q", got)
	}
	if got := autoModeArray(t, settings, "soft_deny"); got != nil {
		t.Errorf("soft_deny = %q, want none: its only entry is for codex", got)
	}

	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	if len(result.Unchanged) != 1 || len(result.Restored) != 0 {
		t.Errorf("reinstall: Unchanged = %v, Restored = %v", result.Unchanged, result.Restored)
	}

	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	settings = mustReadSettings(t, path)
	for _, key := range []string{"environment", "allow"} {
		if got := autoModeArray(t, settings, key); !slices.Equal(got, []any{"$defaults"}) {
			t.Errorf("%s after uninstall = %q, want only $defaults", key, got)
		}
	}
}

// An array the user already wrote is theirs, including leaving out
// $defaults to replace Claude's built-in rules.
func TestApprovalLeavesExistingArrayAsTheUserHasIt(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	path := seedSettings(t, env.TargetDir, `{"autoMode": {"soft_deny": ["mine"]}}`)
	env.SetupAsset("approvals", "a.md", "## deny: No prod\n\nNever touch prod.\n")
	assets := []*config.Asset{approvalAsset(env, "a.md")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if got := autoModeArray(t, mustReadSettings(t, path), "soft_deny"); !slices.Equal(got, []any{"mine", "No prod: Never touch prod."}) {
		t.Errorf("soft_deny = %q, want the user's entry then tai's, no seed", got)
	}

	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	if len(result.Restored) != 0 {
		t.Errorf("reinstall reported Restored = %v; the unseeded array is not a missing entry", result.Restored)
	}

	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if got := autoModeArray(t, mustReadSettings(t, path), "soft_deny"); !slices.Equal(got, []any{"mine"}) {
		t.Errorf("soft_deny after uninstall = %q, want the user's entry", got)
	}
}

func TestApprovalEditedEntryConflicts(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("approvals", "a.md", "## allow: Builds\n\nRun builds.\n")
	assets := []*config.Asset{approvalAsset(env, "a.md")}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	path := filepath.Join(env.TargetDir, "settings.json")
	settings := mustReadSettings(t, path)
	settings["autoMode"].(map[string]any)["allow"].([]any)[1] = "Builds: edited"
	writeJSONFile(t, path, settings)

	_, err := installer.Install(adap, "p", assets, InstallOptions{})
	var conflicts *ConflictsError
	if !errors.As(err, &conflicts) {
		t.Fatalf("err = %v, want a conflict", err)
	}
}

func TestApprovalRejectsScopesClaudeDoesNotRead(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("approvals", "a.md", "## allow: Builds\n\nRun builds.\n")
	asset := approvalAsset(env, "a.md")
	asset.Scope = config.ScopeProject

	_, err := installer.Install(adap, "p", []*config.Asset{asset}, InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "scope: user") {
		t.Fatalf("err = %v, want one pointing at scope: user", err)
	}
}

func TestApprovalRejectsBadHeadings(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	for name, content := range map[string]string{
		"kind.md":   "## permit: Builds\n",
		"target.md": "## allow cladue: Builds\n",
	} {
		env.SetupAsset("approvals", name, content)
		_, err := installer.Install(adap, "p", []*config.Asset{approvalAsset(env, name)}, InstallOptions{})
		if err == nil {
			t.Errorf("%s: install succeeded, want an error", name)
		}
	}
}

func TestApprovalUnsupportedTargetWarnsOnlyWhenAnEntryApplies(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	adap := codex.New(env.TargetDir)

	env.SetupAsset("approvals", "claude-only.md", "## allow claude: Builds\n")
	env.SetupAsset("approvals", "everyone.md", "## allow: Builds\n")

	result, err := installer.Install(adap, "p", []*config.Asset{
		approvalAsset(env, "claude-only.md"),
		approvalAsset(env, "everyone.md"),
	}, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	joined := strings.Join(result.Warnings, "\n")
	if strings.Contains(joined, "claude-only") {
		t.Errorf("warned about an asset with nothing for codex: %v", result.Warnings)
	}
	if !strings.Contains(joined, "everyone.md") {
		t.Errorf("no warning for an asset with an entry for codex: %v", result.Warnings)
	}
}

func TestSettingsRejectsApprovalArrays(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "s.yaml", "native: {claude: {autoMode: {allow: [x]}}}\n")

	_, err := installer.Install(adap, "p", []*config.Asset{settingsAsset(env, "s.yaml")}, InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "approval assets") {
		t.Fatalf("err = %v, want one pointing at approval assets", err)
	}
}
