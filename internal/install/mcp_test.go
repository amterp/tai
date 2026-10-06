package install

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/adapter/opencode"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func mcpAsset(env *testutil.TestEnv, name string) *config.Asset {
	return &config.Asset{
		Type:       config.AssetTypeMCP,
		Name:       name,
		SourceName: "local",
		SourcePath: filepath.Join(env.ConfigRepo, "assets", "mcps", name),
	}
}

// projectScoped marks an asset so it installs into the working directory,
// which keeps Claude MCP tests off the real ~/.claude.json.
func projectScoped(a *config.Asset) *config.Asset {
	a.Scope = config.ScopeProject
	return a
}

func TestMCPStdioInstallsToClaude(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	wd := t.TempDir()
	defer chdir(t, wd)()

	env.SetupAsset("mcps", "github.yaml", `transport: stdio
command: npx
args: ["-y", "@modelcontextprotocol/server-github"]
env:
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
`)

	assets := []*config.Asset{projectScoped(mcpAsset(env, "github.yaml"))}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Project scope uses .mcp.json - a dedicated single-key file.
	doc, err := readJSONFile(filepath.Join(wd, ".mcp.json"))
	if err != nil {
		t.Fatalf("reading .mcp.json: %v", err)
	}
	server := doc["mcpServers"].(map[string]any)["github"].(map[string]any)

	if server["type"] != "stdio" {
		t.Errorf("type = %v, want 'stdio'", server["type"])
	}
	if server["command"] != "npx" {
		t.Errorf("command = %v, want 'npx'", server["command"])
	}
	args := server["args"].([]any)
	if len(args) != 2 || args[0] != "-y" {
		t.Errorf("args = %v", args)
	}
	// Claude interpolates ${VAR} itself, so the reference passes through.
	envMap := server["env"].(map[string]any)
	if envMap["GITHUB_TOKEN"] != "${GITHUB_TOKEN}" {
		t.Errorf("env.GITHUB_TOKEN = %v, want the reference preserved", envMap["GITHUB_TOKEN"])
	}
}

func TestMCPHTTPInstallsToClaude(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	wd := t.TempDir()
	defer chdir(t, wd)()

	env.SetupAsset("mcps", "api.yaml", `transport: http
url: https://api.example.com/mcp
headers:
  Authorization: "Bearer ${API_KEY}"
`)

	assets := []*config.Asset{projectScoped(mcpAsset(env, "api.yaml"))}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	doc, err := readJSONFile(filepath.Join(wd, ".mcp.json"))
	if err != nil {
		t.Fatalf("reading .mcp.json: %v", err)
	}
	server := doc["mcpServers"].(map[string]any)["api"].(map[string]any)
	if server["type"] != "http" {
		t.Errorf("type = %v, want 'http'", server["type"])
	}
	if server["url"] != "https://api.example.com/mcp" {
		t.Errorf("url = %v", server["url"])
	}
	if _, hasCommand := server["command"]; hasCommand {
		t.Error("http server should not carry a command")
	}
}

// The dangerous case: user scope writes into a file holding a session's entire
// mutable state. Every other key must round-trip untouched, including large
// integers that a float64 decode would mangle.
func TestMCPUserScopePreservesSurroundingState(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)

	home := t.TempDir()
	t.Setenv("HOME", home)
	// The Claude adapter resolves ~/.claude.json through the home directory,
	// so it must be built after HOME is redirected.
	adap := claude.New(env.TargetDir)

	claudeJSON := filepath.Join(home, ".claude.json")
	original := `{
  "oauthAccount": { "accountUuid": "abc-123", "emailAddress": "me@example.com" },
  "userID": "user-9",
  "firstStartTime": 1759000000000,
  "numStartups": 9007199254740993,
  "projects": {
    "/Users/me/src/thing": { "hasTrustDialogAccepted": true, "lastCost": 1.25 }
  }
}
`
	if err := os.WriteFile(claudeJSON, []byte(original), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	env.SetupAsset("mcps", "github.yaml", "transport: stdio\ncommand: npx\n")
	assets := []*config.Asset{mcpAsset(env, "github.yaml")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	after, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Large integers must survive exactly - a float64 round trip would render
	// these in scientific notation or lose the last digit.
	for _, literal := range []string{"1759000000000", "9007199254740993", "abc-123", "me@example.com"} {
		if !strings.Contains(string(after), literal) {
			t.Errorf("%s did not survive the rewrite:\n%s", literal, after)
		}
	}

	doc, err := decodeJSON(after)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := doc["mcpServers"].(map[string]any)["github"]; !ok {
		t.Error("the MCP server was not installed")
	}

	// Uninstall must put the file back exactly as it was.
	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	restored, err := readJSONFile(claudeJSON)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, exists := restored["mcpServers"]; exists {
		t.Error("mcpServers key survived uninstall with no servers left")
	}
	originalDoc, err := decodeJSON([]byte(original))
	if err != nil {
		t.Fatalf("parse original: %v", err)
	}
	got, _ := json.Marshal(restored)
	want, _ := json.Marshal(originalDoc)
	if string(got) != string(want) {
		t.Errorf("uninstall did not restore the file.\n got: %s\nwant: %s", got, want)
	}
}

// An MCP server tai installs must not disturb servers already configured.
func TestMCPPreservesForeignServers(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	wd := t.TempDir()
	defer chdir(t, wd)()

	mcpPath := filepath.Join(wd, ".mcp.json")
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"existing":{"command":"keep-me"}}}`), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	env.SetupAsset("mcps", "github.yaml", "transport: stdio\ncommand: npx\n")
	assets := []*config.Asset{projectScoped(mcpAsset(env, "github.yaml"))}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	doc, err := readJSONFile(mcpPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	servers, ok := doc["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers removed entirely, taking the pre-existing server with it")
	}
	if _, exists := servers["existing"]; !exists {
		t.Error("the pre-existing server was removed")
	}
	if _, exists := servers["github"]; exists {
		t.Error("tai's server was not removed")
	}
}

// OpenCode's shape diverges structurally: one argv array, "environment" rather
// than "env", and a required type of local/remote.
func TestMCPTranslatesForOpenCode(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	adap := opencode.New(env.TargetDir)

	env.SetupAsset("mcps", "github.yaml", `transport: stdio
command: npx
args: ["-y", "server-github"]
env:
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
`)

	assets := []*config.Asset{mcpAsset(env, "github.yaml")}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	doc, err := readJSONFile(filepath.Join(env.TargetDir, "opencode.json"))
	if err != nil {
		t.Fatalf("reading opencode.json: %v", err)
	}
	// The key is "mcp", not "mcpServers".
	server := doc["mcp"].(map[string]any)["github"].(map[string]any)

	if server["type"] != "local" {
		t.Errorf("type = %v, want 'local'", server["type"])
	}
	command := server["command"].([]any)
	if len(command) != 3 || command[0] != "npx" || command[2] != "server-github" {
		t.Errorf("command = %v, want the binary and its args as one array", command)
	}
	if _, wrong := server["args"]; wrong {
		t.Error("args should be folded into command for OpenCode")
	}

	// OpenCode interpolates {env:VAR}, not ${VAR}.
	envMap, ok := server["environment"].(map[string]any)
	if !ok {
		t.Fatalf("no 'environment' key; got %v", server)
	}
	if envMap["GITHUB_TOKEN"] != "{env:GITHUB_TOKEN}" {
		t.Errorf("environment.GITHUB_TOKEN = %v, want OpenCode's syntax", envMap["GITHUB_TOKEN"])
	}
}

// Claude has no per-server cwd. Dropping it silently would leave a server
// starting in the wrong directory with no clue why.
func TestMCPCwdWarnsOnClaude(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	defer chdir(t, t.TempDir())()

	env.SetupAsset("mcps", "local.yaml", "transport: stdio\ncommand: my-server\ncwd: ./tools\n")
	assets := []*config.Asset{projectScoped(mcpAsset(env, "local.yaml"))}

	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	var warned bool
	for _, w := range result.Warnings {
		if strings.Contains(w, "cwd") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want one about cwd being dropped", result.Warnings)
	}
}

func TestMCPDisabledServerIsNotInstalled(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	wd := t.TempDir()
	defer chdir(t, wd)()

	env.SetupAsset("mcps", "off.yaml", "transport: stdio\ncommand: npx\nenabled: false\n")
	assets := []*config.Asset{projectScoped(mcpAsset(env, "off.yaml"))}

	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want nothing", result.Added)
	}
	if _, err := os.Stat(filepath.Join(wd, ".mcp.json")); !os.IsNotExist(err) {
		t.Error(".mcp.json was created for a disabled server")
	}
}

func TestMCPMultipleServersCoexist(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	wd := t.TempDir()
	defer chdir(t, wd)()

	env.SetupAsset("mcps", "a.yaml", "transport: stdio\ncommand: first\n")
	env.SetupAsset("mcps", "b.yaml", "transport: stdio\ncommand: second\n")

	assets := []*config.Asset{
		projectScoped(mcpAsset(env, "a.yaml")),
		projectScoped(mcpAsset(env, "b.yaml")),
	}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	doc, err := readJSONFile(filepath.Join(wd, ".mcp.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	servers := doc["mcpServers"].(map[string]any)
	if len(servers) != 2 {
		t.Errorf("servers = %v, want two", servers)
	}
}

func TestMCPValidationRejectsBadConfigs(t *testing.T) {
	cases := []struct {
		name    string
		content string
		errMsg  string
	}{
		{"missing transport", "command: npx\n", "transport"},
		{"unknown transport", "transport: carrier-pigeon\ncommand: x\n", "unknown transport"},
		{"stdio without command", "transport: stdio\n", "command"},
		{"stdio with url", "transport: stdio\ncommand: x\nurl: https://e.com\n", "url"},
		{"http without url", "transport: http\n", "url"},
		{"http with command", "transport: http\nurl: https://e.com\ncommand: x\n", "command"},
		{"http with env", "transport: http\nurl: https://e.com\nenv: {A: b}\n", "env"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, installer, adap := setupStatusLineTest(t)
			defer chdir(t, t.TempDir())()
			env.SetupAsset("mcps", "bad.yaml", tc.content)

			_, err := installer.Install(adap, "p",
				[]*config.Asset{projectScoped(mcpAsset(env, "bad.yaml"))}, InstallOptions{})
			if err == nil {
				t.Fatalf("Install succeeded, want an error mentioning %q", tc.errMsg)
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("error = %v, want it to mention %q", err, tc.errMsg)
			}
		})
	}
}

// Installing the same server twice must not duplicate or churn it.
func TestMCPReinstallIsIdempotent(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	wd := t.TempDir()
	defer chdir(t, wd)()

	env.SetupAsset("mcps", "github.yaml", "transport: stdio\ncommand: npx\n")
	assets := []*config.Asset{projectScoped(mcpAsset(env, "github.yaml"))}
	mcpPath := filepath.Join(wd, ".mcp.json")

	var last string
	for run := 1; run <= 3; run++ {
		if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
			t.Fatalf("install %d failed: %v", run, err)
		}
		data, err := os.ReadFile(mcpPath)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if run > 1 && string(data) != last {
			t.Errorf("install %d changed .mcp.json", run)
		}
		last = string(data)
	}
}

// Claude's "local" scope means a server private to one project. It records that
// under the project's own entry in ~/.claude.json, not in a file in the project
// directory - writing a .claude.json in the repo would produce a file Claude
// never reads, so the server would silently never appear.
func TestMCPLocalScopeNestsUnderProjectRecord(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)

	home := t.TempDir()
	t.Setenv("HOME", home)
	adap := claude.New(env.TargetDir)

	wd := t.TempDir()
	defer chdir(t, wd)()
	// The working directory is the project key, and macOS hands out symlinked
	// temp dirs, so resolve it the same way the adapter does.
	project, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	claudeJSON := filepath.Join(home, ".claude.json")
	seed := `{"projects":{"` + project + `":{"hasTrustDialogAccepted":true}},"userID":"user-9"}`
	if err := os.WriteFile(claudeJSON, []byte(seed), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	env.SetupAsset("mcps", "scratch.yaml", "transport: stdio\ncommand: npx\n")
	asset := mcpAsset(env, "scratch.yaml")
	asset.Scope = config.ScopeLocal

	if _, err := installer.Install(adap, "p", []*config.Asset{asset}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(wd, ".claude.json")); !os.IsNotExist(err) {
		t.Error("a .claude.json was written in the project directory, which Claude never reads")
	}

	doc, err := readJSONFile(claudeJSON)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	record, ok := doc["projects"].(map[string]any)[project].(map[string]any)
	if !ok {
		t.Fatalf("no project record for %q in:\n%v", project, doc)
	}
	if _, ok := record["mcpServers"].(map[string]any)["scratch"]; !ok {
		t.Errorf("server not recorded under projects[%q].mcpServers: %v", project, record)
	}
	if record["hasTrustDialogAccepted"] != true {
		t.Error("the project's existing trust decision was lost")
	}
	if _, exists := doc["mcpServers"]; exists {
		t.Error("a local-scope server leaked into the global mcpServers map")
	}

	// Removal must take the nested map out without touching the rest of the
	// project's record.
	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	after, err := readJSONFile(claudeJSON)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	record = after["projects"].(map[string]any)[project].(map[string]any)
	if _, exists := record["mcpServers"]; exists {
		t.Error("the emptied mcpServers map survived uninstall")
	}
	if record["hasTrustDialogAccepted"] != true {
		t.Error("uninstall took the project's trust decision with it")
	}
	if after["userID"] != "user-9" {
		t.Error("unrelated global state was lost")
	}
}

// When tai creates the project record itself - a project Claude has never
// opened - uninstall should leave no empty husk behind.
func TestMCPLocalScopePrunesRecordItCreated(t *testing.T) {
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)

	home := t.TempDir()
	t.Setenv("HOME", home)
	adap := claude.New(env.TargetDir)

	defer chdir(t, t.TempDir())()

	claudeJSON := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeJSON, []byte(`{"userID":"user-9"}`), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	env.SetupAsset("mcps", "scratch.yaml", "transport: stdio\ncommand: npx\n")
	asset := mcpAsset(env, "scratch.yaml")
	asset.Scope = config.ScopeLocal

	if _, err := installer.Install(adap, "p", []*config.Asset{asset}, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	doc, err := readJSONFile(claudeJSON)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if projects, exists := doc["projects"]; exists {
		t.Errorf("uninstall left an empty projects husk: %v", projects)
	}
	if doc["userID"] != "user-9" {
		t.Error("unrelated state was lost")
	}
}

// Hand-editing a tai-managed server and reinstalling used to silently restore
// tai's version - no conflict, no --force required. Drift means the user has
// taken ownership, and is gated exactly like a modified file asset.
func TestMCPReinstallOverUserEditIsAConflict(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	wd := t.TempDir()
	defer chdir(t, wd)()

	env.SetupAsset("mcps", "github.yaml", "transport: stdio\ncommand: npx\n")
	assets := []*config.Asset{projectScoped(mcpAsset(env, "github.yaml"))}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	mcpPath := filepath.Join(wd, ".mcp.json")
	doc, err := readJSONFile(mcpPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	doc["mcpServers"].(map[string]any)["github"].(map[string]any)["command"] = "bunx"
	writeJSONFile(t, mcpPath, doc)

	_, err = installer.Install(adap, "p", assets, InstallOptions{})
	var conflictErr *ConflictsError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("reinstall over a hand-edited server returned %v, want a conflict", err)
	}

	doc, err = readJSONFile(mcpPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := doc["mcpServers"].(map[string]any)["github"].(map[string]any)["command"]; got != "bunx" {
		t.Errorf("command = %v, want the user's edit preserved", got)
	}

	// --force takes it back.
	if _, err := installer.Install(adap, "p", assets, InstallOptions{Force: true}); err != nil {
		t.Fatalf("forced reinstall failed: %v", err)
	}
	doc, err = readJSONFile(mcpPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := doc["mcpServers"].(map[string]any)["github"].(map[string]any)["command"]; got != "npx" {
		t.Errorf("command = %v, want tai's value restored by --force", got)
	}
}
