package install

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/antigravity"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func TestAntigravityHookOwnership(t *testing.T) {
	env := testutil.NewTestEnv(t)
	adap := antigravity.New(env.TargetDir)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)

	const foreign = `"memri":{"SessionStart":[{"command":"memri hook"}]}`
	env.WriteTargetFile("config/hooks.json", "{"+foreign+"}")
	hookPath := filepath.Join(env.TargetDir, "config", "hooks.json")

	env.SetupAsset("hooks", "on-edit.yaml", "on: pre_tool_use\ntools: [edit]\nrun: inspect-edit\ntimeout: 5s\n")
	env.SetupAsset("hooks", "before-call.yaml", `on: pre_invocation
run: before-call
native:
  antigravity:
    command: before-call-antigravity
`)
	assets := []*config.Asset{hookAsset(env, "on-edit.yaml"), hookAsset(env, "before-call.yaml")}

	install := func(opts InstallOptions) *InstallResult {
		t.Helper()
		result, err := installer.Install(adap, "p", assets, opts)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	install(InstallOptions{DryRun: true})
	assertAntigravityJSON(t, hookPath, "{"+foreign+"}")

	install(InstallOptions{})
	const direct = `"before-call":{"PreInvocation":[{"type":"command","command":"before-call-antigravity"}]}`
	assertAntigravityJSON(t, hookPath, "{"+foreign+","+direct+","+
		`"on-edit":{"PreToolUse":[{"matcher":"edit_file|replace_file_content|multi_replace_file_content",
		"hooks":[{"type":"command","command":"inspect-edit","timeout":5}]}]}}`)

	env.SetupAsset("hooks", "on-edit.yaml", `on: post_tool_use
tools: [edit]
match:
  antigravity: '^write_file$'
run: after-write
`)
	reinstalled := install(InstallOptions{})
	if !slices.Contains(reinstalled.Updated, "[hook] on-edit") {
		t.Errorf("reinstall did not update edited hook asset: %+v", reinstalled)
	}
	const edited = `"on-edit":{"PostToolUse":[{"matcher":"^write_file$",
		"hooks":[{"type":"command","command":"after-write"}]}]}`
	assertAntigravityJSON(t, hookPath, "{"+foreign+","+direct+","+edited+"}")

	const externalEdit = `"on-edit":{"PostToolUse":[{"matcher":"^write_file$","hooks":[{"command":"mine"}]}]}`
	env.WriteTargetFile("config/hooks.json", "{"+foreign+","+direct+","+externalEdit+"}")
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err == nil || !strings.Contains(err.Error(), "modified since last install") {
		t.Fatalf("want conflict on an externally edited group, got %v", err)
	}
	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatal(err)
	}
	assertAntigravityJSON(t, hookPath, "{"+foreign+","+externalEdit+"}")

	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err == nil || !strings.Contains(err.Error(), "unmanaged on-edit") {
		t.Fatalf("want conflict on an unmanaged group with the same name, got %v", err)
	}
	assertAntigravityJSON(t, hookPath, "{"+foreign+","+externalEdit+"}")
}

func TestAntigravityInstallsGlobalFilesAndProjectConfig(t *testing.T) {
	env := testutil.NewTestEnv(t)
	adap := antigravity.New(env.TargetDir)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)

	workspace := t.TempDir()
	t.Chdir(workspace)
	env.SetupAsset("skills", "check/SKILL.md", "---\nname: check\n---\nSkill body\n")
	env.SetupAsset("delegates", "review/DELEGATE.md", `---
name: review
description: Review code
model: pro
native:
  antigravity:
    tools: [view_file]
---
Review body
`)
	env.SetupAsset("delegates", "review/guide.md", "Reference\n")
	env.SetupAsset("files", "instructions.md", "Instructions for {{ target_name_display }}\n")
	env.SetupAsset("hooks", "startup.yaml", "on: session_start\nrun: startup\n")
	env.SetupAsset("mcps", "remote.yaml", `transport: http
url: https://example.com/mcp
headers:
  X-MCP-Test: abc
`)
	env.SetupAsset("mcps", "local.yaml", `transport: stdio
command: mcp-server
args: [--quiet]
env: {MODE: test}
cwd: /project
native:
  antigravity:
    timeoutSeconds: 60
`)
	env.SetupAsset("statuslines", "bar.yaml", "level: custom\ncommand: show-bar\npadding: 0\n")
	env.SetupAsset("settings", "model.yaml", "native: {antigravity: {model: test}}\n")

	projectHook := hookAsset(env, "startup.yaml")
	status := statusLineAsset(env, "bar.yaml")
	status.Scope = config.ScopeUser
	assets := []*config.Asset{
		{Type: config.AssetTypeSkill, Name: "check", IsDir: true, SourcePath: filepath.Join(env.ConfigRepo, "assets", "skills", "check")},
		{Type: config.AssetTypeDelegate, Name: "review", IsDir: true, SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "review")},
		{Type: config.AssetTypeFile, Name: "instructions.md", Dest: "{{ target_agents_file }}", SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "instructions.md")},
		projectHook,
		{Type: config.AssetTypeMCP, Name: "remote.yaml", SourcePath: filepath.Join(env.ConfigRepo, "assets", "mcps", "remote.yaml")},
		{Type: config.AssetTypeMCP, Name: "local.yaml", Scope: config.ScopeUser,
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "mcps", "local.yaml")},
		status,
		{Type: config.AssetTypeSettings, Name: "model.yaml", Scope: config.ScopeUser,
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "settings", "model.yaml")},
	}
	const settingsPath = "antigravity-cli/settings.json"
	const other = `"trustedWorkspaces":["/workspace"],"permissions":{"ask":["write_file(.env)"]}`
	env.WriteTargetFile(settingsPath, "{"+other+"}")

	result, err := installer.Install(adap, "p", assets, InstallOptions{Scope: config.ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != len(assets) || len(result.Warnings) > 0 {
		t.Fatalf("unexpected install result: %+v", result)
	}
	for path, want := range map[string]string{
		"AGENTS.md":                     "Instructions for Antigravity\n",
		"config/skills/check/SKILL.md":  "---\nname: check\ndescription: check\n---\n\nSkill body\n",
		"config/agents/review/agent.md": "tools:\n  - view_file\n",
		"config/agents/review/guide.md": "Reference\n",
	} {
		if got := env.ReadTargetFile(path); !strings.Contains(got, want) {
			t.Errorf("%s = %q, want it to contain %q", path, got, want)
		}
	}
	if env.TargetFileExists("config/agents/review/DELEGATE.md") {
		t.Error("source main filename also copied to target")
	}

	projectHooks := filepath.Join(workspace, ".agents", "hooks.json")
	projectMCP := filepath.Join(workspace, ".agents", "mcp_config.json")
	userMCP := filepath.Join(env.TargetDir, "config", "mcp_config.json")
	assertAntigravityJSON(t, projectHooks, `{"startup":{"SessionStart":[{"type":"command","command":"startup"}]}}`)
	assertAntigravityJSON(t, projectMCP, `{"mcpServers":{"remote":{"serverUrl":"https://example.com/mcp","headers":{"X-MCP-Test":"abc"}}}}`)
	assertAntigravityJSON(t, userMCP, `{"mcpServers":{"local":{"command":"mcp-server","args":["--quiet"],"cwd":"/project","env":{"MODE":"test"},"timeoutSeconds":60}}}`)
	assertAntigravityJSON(t, filepath.Join(env.TargetDir, settingsPath), "{"+other+","+
		`"statusLine":{"type":"command","command":"show-bar","padding":0},"model":"test"}`)

	t.Chdir(t.TempDir())
	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatal(err)
	}
	assertAntigravityJSON(t, projectHooks, `{}`)
	assertAntigravityJSON(t, projectMCP, `{}`)
	assertAntigravityJSON(t, userMCP, `{}`)
	assertAntigravityJSON(t, filepath.Join(env.TargetDir, settingsPath), "{"+other+"}")
	for _, path := range []string{"AGENTS.md", "config/agents/review", "config/skills/check"} {
		if env.TargetFileExists(path) {
			t.Errorf("%s survived uninstall", path)
		}
	}
}

func TestAntigravityDirectoryDelegateRejectsConflictingSidecar(t *testing.T) {
	env := testutil.NewTestEnv(t)
	adap := antigravity.New(env.TargetDir)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	env.SetupAsset("delegates", "review/DELEGATE.md", "---\nname: review\n---\nDelegate\n")
	env.SetupAsset("delegates", "review/agent.md", "Sidecar\n")
	delegate := &config.Asset{
		Type: config.AssetTypeDelegate, Name: "review", IsDir: true,
		SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "review"),
	}
	_, err := installer.Install(adap, "p", []*config.Asset{delegate}, InstallOptions{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "agent.md conflicts with the installed name of DELEGATE.md") {
		t.Fatalf("want main file collision, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.TargetDir, "config", "agents", "review")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected installed directory: %v", err)
	}
}

func assertAntigravityJSON(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %s, want %s", path, data, expected)
	}
}
