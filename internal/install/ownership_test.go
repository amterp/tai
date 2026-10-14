package install

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/amterp/tai/internal/config"
)

func permissionsAllow(t *testing.T, path string) []any {
	t.Helper()
	perms, _ := mustReadSettings(t, path)["permissions"].(map[string]any)
	allow, _ := perms["allow"].([]any)
	return allow
}

func setPermissionsAllow(t *testing.T, path string, allow []any) {
	t.Helper()
	settings := mustReadSettings(t, path)
	settings["permissions"].(map[string]any)["allow"] = allow
	writeJSONFile(t, path, settings)
}

// A field tai wrote changes owner when its asset is renamed. The old entry is
// no longer in the profile, so only the new owner's check can notice the
// user's edit.
func TestRenamedSettingsAssetKeepsUsersEdit(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "old.yaml", "native: {claude: {model: opus}}\n")
	path := filepath.Join(env.TargetDir, "settings.json")
	if _, err := installer.Install(adap, "p", []*config.Asset{settingsAsset(env, "old.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings := mustReadSettings(t, path)
	settings["model"] = "sonnet"
	writeJSONFile(t, path, settings)

	env.SetupAsset("settings", "new.yaml", "native: {claude: {model: opus}}\n")
	_, err := installer.Install(adap, "p", []*config.Asset{settingsAsset(env, "new.yaml")}, InstallOptions{})
	var conflicts *ConflictsError
	if !errors.As(err, &conflicts) {
		t.Fatalf("err = %v, want a conflict on the edited model", err)
	}
	if got := mustReadSettings(t, path)["model"]; got != "sonnet" {
		t.Errorf("model = %v, want the user's edit kept", got)
	}
}

// The old owner stays in the profile here, so its own drift check catches the
// edit; the renamed-asset test covers the case where it does not.
func TestFieldMovedWhileOldOwnerStaysConflicts(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "a.yaml", "native: {claude: {model: opus, env: {A: \"1\"}}}\n")
	path := filepath.Join(env.TargetDir, "settings.json")
	if _, err := installer.Install(adap, "p", []*config.Asset{settingsAsset(env, "a.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings := mustReadSettings(t, path)
	settings["model"] = "sonnet"
	writeJSONFile(t, path, settings)

	env.SetupAsset("settings", "a.yaml", "native: {claude: {env: {A: \"1\"}}}\n")
	env.SetupAsset("settings", "b.yaml", "native: {claude: {model: opus}}\n")
	_, err := installer.Install(adap, "p", []*config.Asset{settingsAsset(env, "a.yaml"), settingsAsset(env, "b.yaml")}, InstallOptions{})
	var conflicts *ConflictsError
	if !errors.As(err, &conflicts) {
		t.Fatalf("err = %v, want a conflict on the edited model", err)
	}
}

// Deleting one of tai's elements (as Claude's /permissions UI does) closes up
// the array, so the recorded position now holds another of tai's elements.
// That is a deletion, not an edit: the element comes back without a conflict.
func TestDeletedArrayElementIsRestored(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "s.yaml", "native: {claude: {permissions: {allow: [a, b, c]}}}\n")
	assets := []*config.Asset{settingsAsset(env, "s.yaml")}
	path := filepath.Join(env.TargetDir, "settings.json")
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	setPermissionsAllow(t, path, []any{"b", "c"})

	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("reinstall after a deletion failed: %v", err)
	}
	if len(result.Restored) != 1 {
		t.Errorf("Restored = %v, want the asset", result.Restored)
	}
	if got := permissionsAllow(t, path); !slices.Equal(got, []any{"b", "c", "a"}) {
		t.Errorf("allow = %v, want a back exactly once", got)
	}

	result, err = installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil || len(result.Unchanged) != 1 {
		t.Errorf("second reinstall: err = %v, Unchanged = %v", err, result.Unchanged)
	}
}

// The guard design decision 9 exists for: an element the user edited is
// still a conflict, never read as deleted and appended a second time.
func TestEditedArrayElementStillConflicts(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "s.yaml", "native: {claude: {permissions: {allow: [a, b]}}}\n")
	assets := []*config.Asset{settingsAsset(env, "s.yaml")}
	path := filepath.Join(env.TargetDir, "settings.json")
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	setPermissionsAllow(t, path, []any{"a-edited", "b"})

	_, err := installer.Install(adap, "p", assets, InstallOptions{})
	var conflicts *ConflictsError
	if !errors.As(err, &conflicts) {
		t.Fatalf("err = %v, want a conflict", err)
	}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{Force: true}); err != nil {
		t.Fatalf("--force failed: %v", err)
	}
	if got := permissionsAllow(t, path); !slices.Equal(got, []any{"a", "b"}) {
		t.Errorf("allow after --force = %v, want a reclaimed in place, not appended", got)
	}
}

// tai wrote [u1 u2 a b] (after the user's two); the user deletes a and adds u3.
// --force must not reclaim positions by number and overwrite u3.
func TestForceDoesNotClobberUsersElementAfterDeletion(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	path := seedSettings(t, env.TargetDir, `{"permissions": {"allow": ["u1", "u2"]}}`)
	env.SetupAsset("settings", "s.yaml", "native: {claude: {permissions: {allow: [a, b]}}}\n")
	assets := []*config.Asset{settingsAsset(env, "s.yaml")}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	setPermissionsAllow(t, path, []any{"u1", "u2", "b", "u3"})

	if _, err := installer.Install(adap, "p", assets, InstallOptions{Force: true}); err != nil {
		t.Fatalf("--force failed: %v", err)
	}
	if got := permissionsAllow(t, path); !slices.Equal(got, []any{"u1", "u2", "b", "u3", "a"}) {
		t.Errorf("allow = %v, want u3 kept and a appended", got)
	}
}

func TestReorderedArrayKeepsNewOrder(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "s.yaml", "native: {claude: {permissions: {allow: [a, b, c]}}}\n")
	assets := []*config.Asset{settingsAsset(env, "s.yaml")}
	path := filepath.Join(env.TargetDir, "settings.json")
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	env.SetupAsset("settings", "s.yaml", "native: {claude: {permissions: {allow: [c, a, b]}}}\n")
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	if got := permissionsAllow(t, path); !slices.Equal(got, []any{"c", "a", "b"}) {
		t.Errorf("allow = %v, want [c a b]", got)
	}

	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil || len(result.Unchanged) != 1 {
		t.Errorf("reinstall: err = %v, Unchanged = %v", err, result.Unchanged)
	}
}

func TestDeletedHookIsRestoredWithoutConflict(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("hooks", "a.yaml", "on: stop\nrun: a.sh\n")
	env.SetupAsset("hooks", "b.yaml", "on: stop\nrun: b.sh\n")
	assets := []*config.Asset{hookAsset(env, "a.yaml"), hookAsset(env, "b.yaml")}
	path := filepath.Join(env.TargetDir, "settings.json")
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings := mustReadSettings(t, path)
	stop := settings["hooks"].(map[string]any)["Stop"].([]any)
	settings["hooks"].(map[string]any)["Stop"] = stop[1:]
	writeJSONFile(t, path, settings)

	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("reinstall after deleting a hook failed: %v", err)
	}
	if len(result.Restored) != 1 {
		t.Errorf("Restored = %v, want the deleted hook", result.Restored)
	}
	if n := countHandlers(t, path, "Stop"); n != 2 {
		t.Errorf("Stop has %d groups, want 2", n)
	}
}

func TestSettingsKeepUnquotedDatesAsWritten(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("settings", "s.yaml", "native: {claude: {env: {SINCE: 2024-01-02}}}\n")
	if _, err := installer.Install(adap, "p", []*config.Asset{settingsAsset(env, "s.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	vars := mustReadSettings(t, filepath.Join(env.TargetDir, "settings.json"))["env"].(map[string]any)
	if vars["SINCE"] != "2024-01-02" {
		t.Errorf("SINCE = %#v, want the text as written", vars["SINCE"])
	}
}
