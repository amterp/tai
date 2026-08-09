package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/codex"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
	"github.com/amterp/tai/internal/testutil"
)

// grantAll approves every consent request, standing in for a user who said yes.
func grantAll(configasset.ConsentRequest) (bool, error) { return true, nil }

func setupCodexMCP(t *testing.T) (*testutil.TestEnv, *Installer, *codex.Adapter) {
	t.Helper()
	env := testutil.NewTestEnv(t)
	installer := NewInstaller(NewManifestStore(filepath.Join(env.ConfigDir, "manifests")), env.ConfigRepo, nil)
	return env, installer, codex.New(env.TargetDir)
}

// Codex has no dedicated MCP file, so servers go into config.toml - which also
// holds settings tai does not own.
func TestCodexMCPPreservesSurroundingSettings(t *testing.T) {
	env, installer, adap := setupCodexMCP(t)

	tomlPath := filepath.Join(env.TargetDir, "config.toml")
	seed := `model = "gpt-5"
notify = ["/some/notifier", "turn-ended"]

[projects."/Users/me/src/thing"]
trust_level = "trusted"
`
	if err := os.WriteFile(tomlPath, []byte(seed), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	env.SetupAsset("mcps", "github.yaml", "transport: stdio\ncommand: npx\nargs: [\"-y\", \"server\"]\n")
	assets := []*config.Asset{mcpAsset(env, "github.yaml")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{Consent: grantAll}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	data, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(data)

	for _, want := range []string{`model = 'gpt-5'`, "trust_level", "mcp_servers", "npx"} {
		if !strings.Contains(got, want) {
			t.Errorf("config.toml missing %q:\n%s", want, got)
		}
	}

	// Uninstall must take the server back out and leave the rest.
	if _, err := installer.Uninstall(adap); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	data, err = os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got = string(data)
	if strings.Contains(got, "mcp_servers") {
		t.Errorf("mcp_servers survived uninstall:\n%s", got)
	}
	if !strings.Contains(got, "trust_level") {
		t.Errorf("unrelated settings were lost:\n%s", got)
	}
}

// Without consent, nothing is written at all.
func TestCodexMCPWithoutConsentIsSkipped(t *testing.T) {
	env, installer, adap := setupCodexMCP(t)

	env.SetupAsset("mcps", "github.yaml", "transport: stdio\ncommand: npx\n")
	assets := []*config.Asset{mcpAsset(env, "github.yaml")}

	// Nil Consent declines - the safe default for any non-interactive caller.
	result, err := installer.Install(adap, "p", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want nothing installed", result.Added)
	}
	if _, err := os.Stat(filepath.Join(env.TargetDir, "config.toml")); !os.IsNotExist(err) {
		t.Error("config.toml was written despite consent being declined")
	}
	if len(result.Manifest.ConfigEntries) != 0 {
		t.Errorf("manifest recorded %d entries for a declined write", len(result.Manifest.ConfigEntries))
	}

	var warned bool
	for _, w := range result.Warnings {
		if strings.Contains(w, "declined") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want one saying the write was declined", result.Warnings)
	}
}

// Several servers writing the same file should ask once, not once each.
func TestCodexMCPAsksOncePerFile(t *testing.T) {
	env, installer, adap := setupCodexMCP(t)

	env.SetupAsset("mcps", "a.yaml", "transport: stdio\ncommand: first\n")
	env.SetupAsset("mcps", "b.yaml", "transport: stdio\ncommand: second\n")
	assets := []*config.Asset{mcpAsset(env, "a.yaml"), mcpAsset(env, "b.yaml")}

	asked := 0
	opts := InstallOptions{Consent: func(configasset.ConsentRequest) (bool, error) {
		asked++
		return true, nil
	}}

	if _, err := installer.Install(adap, "p", assets, opts); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if asked != 1 {
		t.Errorf("consent asked %d times, want 1", asked)
	}
}

// Codex takes environment variable names, not interpolated strings, so a
// whole-value self-reference becomes an env_vars forward.
func TestCodexMCPEnvForwarding(t *testing.T) {
	env, installer, adap := setupCodexMCP(t)

	env.SetupAsset("mcps", "github.yaml", `transport: stdio
command: npx
env:
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
  LOG_LEVEL: debug
`)
	assets := []*config.Asset{mcpAsset(env, "github.yaml")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{Consent: grantAll}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(env.TargetDir, "config.toml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(data)

	if !strings.Contains(got, "env_vars") || !strings.Contains(got, "GITHUB_TOKEN") {
		t.Errorf("expected GITHUB_TOKEN forwarded via env_vars:\n%s", got)
	}
	if !strings.Contains(got, "LOG_LEVEL") || !strings.Contains(got, "debug") {
		t.Errorf("expected the literal env value to be kept:\n%s", got)
	}
	// The unresolvable reference must never be written literally.
	if strings.Contains(got, "${GITHUB_TOKEN}") {
		t.Errorf("a ${VAR} reference was written literally, which Codex will not expand:\n%s", got)
	}
}

// An Authorization: Bearer ${VAR} header has a dedicated Codex field.
func TestCodexMCPBearerToken(t *testing.T) {
	env, installer, adap := setupCodexMCP(t)

	env.SetupAsset("mcps", "api.yaml", `transport: http
url: https://api.example.com/mcp
headers:
  Authorization: "Bearer ${API_KEY}"
`)
	assets := []*config.Asset{mcpAsset(env, "api.yaml")}

	if _, err := installer.Install(adap, "p", assets, InstallOptions{Consent: grantAll}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(env.TargetDir, "config.toml"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "bearer_token_env_var") || !strings.Contains(got, "API_KEY") {
		t.Errorf("expected bearer_token_env_var = 'API_KEY':\n%s", got)
	}
	if strings.Contains(got, "${API_KEY}") {
		t.Errorf("the reference was written literally:\n%s", got)
	}
}

// References Codex genuinely cannot express must fail loudly rather than
// install a server that authenticates with a literal dollar sign.
func TestCodexMCPUnexpressibleEnvRefIsAnError(t *testing.T) {
	cases := []struct {
		name    string
		content string
		errMsg  string
	}{
		{
			name:    "env key differs from variable",
			content: "transport: stdio\ncommand: npx\nenv:\n  TOKEN: \"${GITHUB_TOKEN}\"\n",
			errMsg:  "own name",
		},
		{
			name:    "embedded reference in env",
			content: "transport: stdio\ncommand: npx\nenv:\n  URL: \"https://${HOST}/x\"\n",
			errMsg:  "embeds an environment reference",
		},
		{
			name:    "embedded reference in a non-bearer header",
			content: "transport: http\nurl: https://e.com\nheaders:\n  X-Key: \"prefix-${KEY}\"\n",
			errMsg:  "embeds an environment reference",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, installer, adap := setupCodexMCP(t)
			env.SetupAsset("mcps", "bad.yaml", tc.content)

			_, err := installer.Install(adap, "p",
				[]*config.Asset{mcpAsset(env, "bad.yaml")}, InstallOptions{Consent: grantAll})
			if err == nil {
				t.Fatalf("Install succeeded, want an error mentioning %q", tc.errMsg)
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("error = %v, want it to mention %q", err, tc.errMsg)
			}
		})
	}
}

// Hooks and MCP servers on Codex live in different files, and only the TOML
// one needs consent.
func TestCodexHooksNeedNoConsent(t *testing.T) {
	env, installer, adap := setupCodexMCP(t)

	env.SetupAsset("hooks", "guard.yaml", "on: pre_tool_use\nrun: guard.sh\n")

	// Nil Consent: a hook must still install, since hooks.json is tai's to write.
	result, err := installer.Install(adap, "p", []*config.Asset{hookAsset(env, "guard.yaml")}, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("Added = %v, want the hook installed without asking", result.Added)
	}
	if _, err := os.Stat(filepath.Join(env.TargetDir, "hooks.json")); err != nil {
		t.Errorf("hooks.json not written: %v", err)
	}
}
