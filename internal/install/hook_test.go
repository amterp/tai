package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/codex"
	"github.com/amterp/tai/internal/adapter/gemini"
	"github.com/amterp/tai/internal/adapter/opencode"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func hookAsset(env *testutil.TestEnv, name string) *config.Asset {
	return &config.Asset{
		Type:       config.AssetTypeHook,
		Name:       name,
		SourceName: "local",
		SourcePath: filepath.Join(env.ConfigRepo, "assets", "hooks", name),
	}
}

// seedForeignHooks writes a settings.json holding hooks from three owners that
// are not tai, mirroring a real ~/.claude/settings.json.
func seedForeignHooks(t *testing.T, targetDir string) string {
	t.Helper()
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(targetDir, "settings.json")
	content := `{
  "hooks": {
    "Stop": [
      { "hooks": [{ "type": "command", "command": "afplay /System/Library/Sounds/Glass.aiff" }] },
      { "matcher": "", "hooks": [{ "type": "command", "command": "/peon-ping/peon.sh", "timeout": 10, "async": true }] }
    ],
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "memri hook claude-code session-start" }] }
    ]
  }
}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return path
}

// countHandlers returns how many matcher groups sit under an event.
func countHandlers(t *testing.T, settingsPath, event string) int {
	t.Helper()
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return 0
	}
	arr, ok := hooks[event].([]any)
	if !ok {
		return 0
	}
	return len(arr)
}

func TestHookInstallsToClaude(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)

	env.SetupAsset("hooks", "format.yaml", `description: Format after edits
on: post_tool_use
tools: [edit, write]
run: gofmt -w "$FILE"
timeout: 30s
`)

	result, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "format.yaml")}, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Added) != 1 {
		t.Fatalf("Added = %v, want one entry", result.Added)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	group := settings["hooks"].(map[string]any)["PostToolUse"].([]any)[0].(map[string]any)
	if group["matcher"] != "Edit|Write" {
		t.Errorf("matcher = %v, want 'Edit|Write' (canonical tools translated)", group["matcher"])
	}

	handler := group["hooks"].([]any)[0].(map[string]any)
	if handler["type"] != "command" {
		t.Errorf("type = %v, want 'command'", handler["type"])
	}
	if handler["command"] != `gofmt -w "$FILE"` {
		t.Errorf("command = %v", handler["command"])
	}
	// Claude takes seconds.
	if got := jsonNumber(t, handler["timeout"]); got != "30" {
		t.Errorf("timeout = %v, want 30 (seconds)", handler["timeout"])
	}
}

// The whole point of the ownership rework: tai's hook joins entries written by
// other tools, and leaves with only its own.
func TestHookInstallUninstallPreservesForeignHooks(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	settingsPath := seedForeignHooks(t, env.TargetDir)

	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var beforeDoc map[string]any
	if err := json.Unmarshal(before, &beforeDoc); err != nil {
		t.Fatalf("parse: %v", err)
	}

	env.SetupAsset("hooks", "chime.yaml", "on: stop\nrun: say done\n")

	if _, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "chime.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if got := countHandlers(t, settingsPath, "Stop"); got != 3 {
		t.Fatalf("Stop groups after install = %d, want 3 (two foreign plus tai's)", got)
	}

	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	afterDoc, err := readJSONFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	// Compare semantically: foreign hooks must be exactly as they started.
	afterJSON, _ := json.Marshal(afterDoc)
	beforeJSON, _ := json.Marshal(beforeDoc)
	if string(afterJSON) != string(beforeJSON) {
		t.Errorf("uninstall did not restore foreign hooks.\n got: %s\nwant: %s", afterJSON, beforeJSON)
	}
}

// Several hooks in one profile must all install - the old type-keyed install
// path would have collapsed them to one.
func TestMultipleHooksCoexist(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)

	env.SetupAsset("hooks", "a.yaml", "on: stop\nrun: first\n")
	env.SetupAsset("hooks", "b.yaml", "on: stop\nrun: second\n")
	env.SetupAsset("hooks", "c.yaml", "on: session_start\nrun: third\n")

	assets := []*config.Asset{hookAsset(env, "a.yaml"), hookAsset(env, "b.yaml"), hookAsset(env, "c.yaml")}
	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Added) != 3 {
		t.Fatalf("Added = %v, want three entries", result.Added)
	}

	settingsPath := filepath.Join(env.TargetDir, "settings.json")
	if got := countHandlers(t, settingsPath, "Stop"); got != 2 {
		t.Errorf("Stop groups = %d, want 2", got)
	}
	if got := countHandlers(t, settingsPath, "SessionStart"); got != 1 {
		t.Errorf("SessionStart groups = %d, want 1", got)
	}

	if len(result.Manifest.ConfigEntries) != 3 {
		t.Errorf("manifest config entries = %d, want 3", len(result.Manifest.ConfigEntries))
	}
}

// Reinstalling must update tai's entry in place rather than appending a copy.
func TestHookReinstallDoesNotAccumulate(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	settingsPath := filepath.Join(env.TargetDir, "settings.json")
	env.SetupAsset("hooks", "chime.yaml", "on: stop\nrun: say done\n")
	assets := []*config.Asset{hookAsset(env, "chime.yaml")}

	for run := 1; run <= 3; run++ {
		if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
			t.Fatalf("install %d failed: %v", run, err)
		}
		if got := countHandlers(t, settingsPath, "Stop"); got != 1 {
			t.Fatalf("after install %d: Stop groups = %d, want 1", run, got)
		}
	}

	// Changing the command updates in place, still one entry.
	env.SetupAsset("hooks", "chime.yaml", "on: stop\nrun: say finished\n")
	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("update install failed: %v", err)
	}
	if len(result.Updated) != 1 {
		t.Errorf("Updated = %v, want one entry", result.Updated)
	}
	if got := countHandlers(t, settingsPath, "Stop"); got != 1 {
		t.Errorf("Stop groups after update = %d, want 1", got)
	}
}

// Dropping a hook from the profile removes it on the next install.
func TestHookRemovedWhenDroppedFromProfile(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	settingsPath := filepath.Join(env.TargetDir, "settings.json")

	env.SetupAsset("hooks", "a.yaml", "on: stop\nrun: first\n")
	env.SetupAsset("hooks", "b.yaml", "on: stop\nrun: second\n")

	both := []*config.Asset{hookAsset(env, "a.yaml"), hookAsset(env, "b.yaml")}
	if _, err := installer.Install(adap, "p", both, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	only := []*config.Asset{hookAsset(env, "a.yaml")}
	result, err := installer.Install(adap, "p", only, InstallOptions{})
	if err != nil {
		t.Fatalf("second Install failed: %v", err)
	}
	if len(result.Removed) != 1 {
		t.Errorf("Removed = %v, want one entry", result.Removed)
	}
	if got := countHandlers(t, settingsPath, "Stop"); got != 1 {
		t.Errorf("Stop groups = %d, want 1", got)
	}
}

// Native fields are the escape hatch for target-specific options tai does not
// abstract. `async` is a real one - the peon-ping hooks use it.
func TestHookNativeFieldsArePassedThrough(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)

	env.SetupAsset("hooks", "ping.yaml", `on: stop
run: peon.sh
native:
  claude:
    async: true
  gemini:
    description: ignored here
`)

	if _, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "ping.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	group := settings["hooks"].(map[string]any)["Stop"].([]any)[0].(map[string]any)
	handler := group["hooks"].([]any)[0].(map[string]any)

	if handler["async"] != true {
		t.Errorf("async = %v, want true", handler["async"])
	}
	if _, leaked := handler["description"]; leaked {
		t.Error("another target's native fields leaked into Claude's handler")
	}
}

// A raw per-target matcher covers patterns the canonical tool vocabulary
// cannot express, such as MCP tools.
func TestHookRawMatcherOverridesTools(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)

	env.SetupAsset("hooks", "mcp.yaml", `on: pre_tool_use
tools: [edit]
run: guard.sh
match:
  claude: "mcp__memory__.*"
`)

	if _, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "mcp.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	group := settings["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	if group["matcher"] != "mcp__memory__.*" {
		t.Errorf("matcher = %v, want the raw per-target value", group["matcher"])
	}
}

// No tools means no matcher at all, which every target reads as "everything".
func TestHookWithoutToolsOmitsMatcher(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("hooks", "all.yaml", "on: pre_tool_use\nrun: audit.sh\n")

	if _, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "all.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	group := settings["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	if _, present := group["matcher"]; present {
		t.Errorf("matcher present (%v), want it omitted", group["matcher"])
	}
}

// Gemini renames every event and takes milliseconds, so it is the real test of
// the canonical vocabulary.
func TestHookTranslatesForGemini(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	adap := gemini.New(env.TargetDir)

	env.SetupAsset("hooks", "guard.yaml", `on: pre_tool_use
tools: [edit, shell]
run: guard.sh
timeout: 5s
`)

	if _, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "guard.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	// pre_tool_use is "BeforeTool" here, not "PreToolUse".
	hooks := settings["hooks"].(map[string]any)
	if _, wrong := hooks["PreToolUse"]; wrong {
		t.Error("Claude's event name was written to Gemini")
	}
	group, ok := hooks["BeforeTool"].([]any)
	if !ok {
		t.Fatalf("no BeforeTool event; got keys %v", hooks)
	}

	first := group[0].(map[string]any)
	if first["matcher"] != "replace|run_shell_command" {
		t.Errorf("matcher = %v, want Gemini's tool names", first["matcher"])
	}

	handler := first["hooks"].([]any)[0].(map[string]any)
	// Gemini takes milliseconds, unlike Claude and Codex.
	if got := jsonNumber(t, handler["timeout"]); got != "5000" {
		t.Errorf("timeout = %v, want 5000 (milliseconds)", handler["timeout"])
	}
}

// An event a target lacks skips that one hook with a warning, rather than
// failing an install that may be targeting several tools.
func TestHookUnsupportedEventSkipsWithWarning(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	adap := gemini.New(env.TargetDir)

	// Gemini has no subagent events.
	env.SetupAsset("hooks", "sub.yaml", "on: subagent_stop\nrun: note.sh\n")

	result, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "sub.yaml")}, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want nothing installed", result.Added)
	}
	if len(result.Manifest.ConfigEntries) != 0 {
		t.Errorf("manifest recorded %d entries for a skipped hook, want 0",
			len(result.Manifest.ConfigEntries))
	}

	var found bool
	for _, w := range result.Warnings {
		if strings.Contains(w, "subagent_stop") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v, want one naming the unsupported event", result.Warnings)
	}
}

func TestHookInstallsToCodex(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	adap := codex.New(env.TargetDir)

	env.SetupAsset("hooks", "guard.yaml", "on: pre_tool_use\ntools: [shell]\nrun: guard.sh\n")

	if _, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "guard.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Codex hooks go to a dedicated hooks.json, never config.toml.
	settings, err := readJSONFile(filepath.Join(env.TargetDir, "hooks.json"))
	if err != nil {
		t.Fatalf("reading hooks.json: %v", err)
	}
	group := settings["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	if group["matcher"] != "Bash" {
		t.Errorf("matcher = %v, want 'Bash'", group["matcher"])
	}
}

// OpenCode has no declarative hooks, so hook assets are skipped entirely.
func TestHookUnsupportedOnOpenCode(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)

	adap := opencode.New(env.TargetDir)

	env.SetupAsset("hooks", "chime.yaml", "on: stop\nrun: say done\n")

	result, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "chime.yaml")}, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want nothing", result.Added)
	}

	var warned bool
	for _, w := range result.Warnings {
		if strings.Contains(w, "hook") && strings.Contains(w, "opencode") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want one about hooks being unsupported", result.Warnings)
	}
}

// An unmapped tool must fail loudly. Silently dropping it would widen the
// matcher to every tool, so a hook meant to guard edits would fire on reads.
func TestHookUnmappedToolIsAnError(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	adap := codex.New(env.TargetDir)

	// Codex has no established name for the canonical "read" tool.
	env.SetupAsset("hooks", "reads.yaml", "on: pre_tool_use\ntools: [read]\nrun: audit.sh\n")

	_, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "reads.yaml")}, InstallOptions{})
	if err == nil {
		t.Fatal("Install succeeded, want an error about the unmapped tool")
	}
	if !strings.Contains(err.Error(), "match") {
		t.Errorf("error = %v, want it to point at the `match` escape hatch", err)
	}
}

func TestHookValidationRejectsBadConfigs(t *testing.T) {
	cases := []struct {
		name    string
		content string
		errMsg  string
	}{
		{"missing event", "run: echo hi\n", "on"},
		{"unknown event", "on: whenever\nrun: echo hi\n", "unknown event"},
		{"missing command", "on: stop\n", "run"},
		{"unknown tool", "on: pre_tool_use\ntools: [teleport]\nrun: x\n", "unknown tool"},
		{"bad timeout", "on: stop\nrun: x\ntimeout: soon\n", "timeout"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, installer, adap := setupStatusLineTest(t)
			env.SetupAsset("hooks", "bad.yaml", tc.content)

			_, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "bad.yaml")}, InstallOptions{})
			if err == nil {
				t.Fatalf("Install succeeded, want an error mentioning %q", tc.errMsg)
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("error = %v, want it to mention %q", err, tc.errMsg)
			}
		})
	}
}

// A sub-second timeout must not truncate to zero, which targets read as either
// "no timeout" or "expire immediately".
func TestHookSubSecondTimeoutRoundsUp(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("hooks", "quick.yaml", "on: stop\nrun: x\ntimeout: 100ms\n")

	if _, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "quick.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	group := settings["hooks"].(map[string]any)["Stop"].([]any)[0].(map[string]any)
	handler := group["hooks"].([]any)[0].(map[string]any)
	if got := jsonNumber(t, handler["timeout"]); got != "1" {
		t.Errorf("timeout = %v, want 1 second (rounded up from 100ms)", handler["timeout"])
	}
}

// Claude hooks and a status line share settings.json without disturbing each
// other.
func TestHookAndStatusLineShareSettingsFile(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)

	env.SetupAsset("statuslines", "bar.yaml", "level: minimal\n")
	env.SetupAsset("hooks", "chime.yaml", "on: stop\nrun: say done\n")

	assets := []*config.Asset{statusLineAsset(env, "bar.yaml"), hookAsset(env, "chime.yaml")}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	if _, ok := settings["statusLine"]; !ok {
		t.Error("statusLine missing")
	}
	if _, ok := settings["hooks"]; !ok {
		t.Error("hooks missing")
	}

	// Dropping the hook leaves the status line alone.
	if _, err := installer.Install(adap, "p", assets[:1], InstallOptions{}); err != nil {
		t.Fatalf("second Install failed: %v", err)
	}
	settings, err = readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	if _, ok := settings["statusLine"]; !ok {
		t.Error("statusLine was removed along with the hook")
	}
	if _, ok := settings["hooks"]; ok {
		t.Error("hooks key should be pruned once tai's only hook is gone")
	}
}

func TestHookIsTemplated(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("hooks", "tmpl.yaml", "on: session_start\nrun: notify {{ target_name }}\n")

	if _, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "tmpl.yaml")}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err := readJSONFile(filepath.Join(env.TargetDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}
	group := settings["hooks"].(map[string]any)["SessionStart"].([]any)[0].(map[string]any)
	handler := group["hooks"].([]any)[0].(map[string]any)
	if handler["command"] != "notify claude" {
		t.Errorf("command = %v, want the target variable substituted", handler["command"])
	}
}

// jsonNumber renders a decoded JSON number for comparison, since config files
// decode with UseNumber.
func jsonNumber(t *testing.T, v any) string {
	t.Helper()
	if v == nil {
		return "<nil>"
	}
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	return strings.TrimSuffix(strings.TrimSuffix(
		strings.ReplaceAll(jsonMarshal(t, v), "\"", ""), ".0"), ".00")
}

func jsonMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
