package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
	"github.com/pelletier/go-toml/v2"
)

func TestAdapterName(t *testing.T) {
	a := New("")
	if a.Name() != "codex" {
		t.Errorf("Name() = %q, want 'codex'", a.Name())
	}
}

func TestAdapterAssetMapping(t *testing.T) {
	a := New("")
	mapping := a.AssetMapping()

	expected := map[string]string{
		"persona":  "skills",
		"delegate": "agents",
		"skill":    "skills",
	}

	for k, v := range expected {
		if mapping[k] != v {
			t.Errorf("mapping[%q] = %q, want %q", k, mapping[k], v)
		}
	}

	// Personas and skills should both map to skills/ since Codex has no native
	// persona concept.
	if mapping["persona"] != mapping["skill"] {
		t.Errorf("persona and skill should both map to 'skills', got %q and %q",
			mapping["persona"], mapping["skill"])
	}
}

func TestAdapterTargetDir(t *testing.T) {
	// Test with explicit target dir
	a := New("/custom/path")
	if a.TargetDir() != "/custom/path" {
		t.Errorf("TargetDir() = %q, want '/custom/path'", a.TargetDir())
	}

	// Test CODEX_HOME override
	t.Setenv("CODEX_HOME", "/env/codex")
	a2 := New("")
	if a2.TargetDir() != "/env/codex" {
		t.Errorf("TargetDir() = %q, want '/env/codex'", a2.TargetDir())
	}

	// Test default fallback
	t.Setenv("CODEX_HOME", "")
	a3 := New("")
	home, err := os.UserHomeDir()
	if err == nil {
		expected := filepath.Join(home, ".codex")
		if a3.TargetDir() != expected {
			t.Errorf("TargetDir() = %q, want %q", a3.TargetDir(), expected)
		}
	}
}

func TestAdapterMapAssetPath(t *testing.T) {
	a := New("")

	tests := []struct {
		asset    *config.Asset
		expected string
	}{
		{
			// Single-file persona wrapped in directory for Codex
			asset:    &config.Asset{Type: "persona", Name: "senior-swe.md"},
			expected: "skills/senior-swe/SKILL.md",
		},
		{
			// A single-file delegate becomes a custom agent TOML file
			asset:    &config.Asset{Type: "delegate", Name: "code-reviewer.md"},
			expected: "agents/code-reviewer.toml",
		},
		{
			// A directory delegate has no TOML form and stays a passive reference
			asset:    &config.Asset{Type: "delegate", Name: "researcher", IsDir: true},
			expected: "delegates/researcher",
		},
		{
			// Single-file skill also wrapped in directory
			asset:    &config.Asset{Type: "skill", Name: "commit.md"},
			expected: "skills/commit/SKILL.md",
		},
		{
			// Directory skill stays as-is
			asset:    &config.Asset{Type: "skill", Name: "my-skill", IsDir: true},
			expected: "skills/my-skill",
		},
		{
			// File type with empty dest goes to root
			asset:    &config.Asset{Type: "file", Name: "AGENTS.md", Dest: ""},
			expected: "AGENTS.md",
		},
		{
			// File type with directory dest (no extension) goes to subdirectory
			asset:    &config.Asset{Type: "file", Name: "config.json", Dest: "config"},
			expected: "config/config.json",
		},
		{
			// File type with filename dest (has extension) uses dest as full path
			asset:    &config.Asset{Type: "file", Name: "rules.md", Dest: "AGENTS.md"},
			expected: "AGENTS.md",
		},
		{
			// File type with path+filename dest uses dest as full path
			asset:    &config.Asset{Type: "file", Name: "settings.yaml", Dest: "config/app.yaml"},
			expected: "config/app.yaml",
		},
		{
			// File type with trailing slash dest is explicitly a directory
			asset:    &config.Asset{Type: "file", Name: "data.json", Dest: "output/"},
			expected: "output/data.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := a.MapAssetPath(tt.asset)
			if result != tt.expected {
				t.Errorf("MapAssetPath(%+v) = %q, want %q", tt.asset, result, tt.expected)
			}
		})
	}
}

func TestAdapterMapAssetPathUnknownType(t *testing.T) {
	a := New("")

	asset := &config.Asset{Type: "unknown", Name: "test.md"}
	result := a.MapAssetPath(asset)

	expected := "unknown/test.md"
	if result != expected {
		t.Errorf("MapAssetPath for unknown type = %q, want %q", result, expected)
	}
}

func TestAdapterMapAssetPathPersonaDirectory(t *testing.T) {
	a := New("")
	// Directory persona stays as directory (no SKILL.md wrapping)
	asset := &config.Asset{Type: "persona", Name: "senior-swe", IsDir: true}
	result := a.MapAssetPath(asset)
	expected := "skills/senior-swe"
	if result != expected {
		t.Errorf("MapAssetPath(dir persona) = %q, want %q", result, expected)
	}
}

func TestAdapterTransformPersona(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypePersona},
		Metadata: map[string]string{
			"name":        "Senior SWE",
			"description": "A senior software engineer persona",
		},
		Content: []byte("# Senior Software Engineer\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\nname: Senior SWE\ndescription: A senior software engineer persona\n---\n\n# Senior Software Engineer\n"
	if string(got) != want {
		t.Errorf("persona transform mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

// transformDelegate runs a single-file delegate through the adapter and
// returns the installed bytes and the warnings it raised.
func transformDelegate(t *testing.T, metadata map[string]string, native map[string]any, body string) (string, []string) {
	t.Helper()
	asset := &config.ResolvedAsset{
		Asset:    config.Asset{Type: config.AssetTypeDelegate, Name: "code-reviewer.md"},
		Metadata: metadata,
		Content:  []byte(body),
	}
	if native != nil {
		asset.Native = frontmatter.Native{"codex": native}
	}
	got, err := New("").TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}
	return string(got), asset.Warnings
}

// parseAgent decodes an installed agent file, failing the test if Codex
// could not parse it either.
func parseAgent(t *testing.T, content string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("installed agent is not valid TOML: %v\n%s", err, content)
	}
	return parsed
}

func TestAdapterTransformDelegateAsAgent(t *testing.T) {
	got, warnings := transformDelegate(t, map[string]string{
		"name":        "code-reviewer",
		"description": "Reviews code for quality",
		"model":       "gpt-5",
	}, nil, "You are a code reviewer.\n")

	want := "name = 'code-reviewer'\n" +
		"description = 'Reviews code for quality'\n" +
		"model = 'gpt-5'\n" +
		"developer_instructions = \"\"\"\nYou are a code reviewer.\n\"\"\"\n"
	if got != want {
		t.Errorf("agent mismatch:\n got: %q\nwant: %q", got, want)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
}

func TestAdapterTransformDelegateBodyRoundTrips(t *testing.T) {
	// Characters that need escaping in a TOML multi-line string must come
	// back unchanged, or Codex runs different instructions than were written.
	body := "Quote: \"\"\" and ''' and a path C:\\tmp\\x.\nTab\there.\n"
	got, _ := transformDelegate(t, map[string]string{"name": "r", "description": "d"}, nil, body)

	if parsed := parseAgent(t, got); parsed["developer_instructions"] != body {
		t.Errorf("developer_instructions = %q, want %q", parsed["developer_instructions"], body)
	}
}

func TestAdapterTransformDelegateModel(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		native    map[string]any
		wantModel any // nil means no model key
		wantWarn  bool
	}{
		{name: "codex model kept", model: "gpt-5", wantModel: "gpt-5"},
		{name: "claude alias dropped", model: "opus", wantWarn: true},
		{name: "claude alias with context suffix dropped", model: "Sonnet[1m]", wantWarn: true},
		{name: "claude model id dropped", model: "claude-opus-5-5", wantWarn: true},
		{name: "inherit dropped silently", model: "inherit"},
		{name: "native model replaces claude alias", model: "opus", native: map[string]any{"model": "gpt-5"}, wantModel: "gpt-5"},
		{name: "native model replaces codex model", model: "gpt-4", native: map[string]any{"model": "gpt-5"}, wantModel: "gpt-5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings := transformDelegate(t, map[string]string{
				"name": "r", "description": "d", "model": tt.model,
			}, tt.native, "Body.\n")

			if model := parseAgent(t, got)["model"]; model != tt.wantModel {
				t.Errorf("model = %v, want %v", model, tt.wantModel)
			}
			if warned := containsSubstring(warnings, "is a Claude model"); warned != tt.wantWarn {
				t.Errorf("model warning = %v, want %v; warnings: %q", warned, tt.wantWarn, warnings)
			}
		})
	}
}

func TestAdapterTransformDelegateTools(t *testing.T) {
	metadata := map[string]string{"name": "r", "description": "d", "tools": "Read, Grep"}

	got, warnings := transformDelegate(t, metadata, nil, "Body.\n")
	if strings.Contains(got, "tools") {
		t.Errorf("tools should not reach a Codex agent:\n%s", got)
	}
	if !containsSubstring(warnings, "no tool allowlist") {
		t.Errorf("missing tools warning, warnings: %q", warnings)
	}

	// Restricting the agent in Codex's own terms answers the warning.
	_, warnings = transformDelegate(t, metadata,
		map[string]any{"features": map[string]any{"shell_tool": false}}, "Body.\n")
	if containsSubstring(warnings, "no tool allowlist") {
		t.Errorf("tools warning should be silenced by native.codex.features, warnings: %q", warnings)
	}
}

func TestAdapterTransformDelegateNative(t *testing.T) {
	got, _ := transformDelegate(t, map[string]string{
		"name": "r", "description": "generated",
	}, map[string]any{
		"description":            "native wins",
		"model_reasoning_effort": "high",
		"features":               map[string]any{"shell_tool": false},
	}, "Body.\n")

	parsed := parseAgent(t, got)
	if parsed["description"] != "native wins" {
		t.Errorf("description = %v, want the native value", parsed["description"])
	}
	if parsed["model_reasoning_effort"] != "high" {
		t.Errorf("model_reasoning_effort = %v, want high", parsed["model_reasoning_effort"])
	}
	if parsed["developer_instructions"] != "Body.\n" {
		t.Errorf("developer_instructions = %v, want the body", parsed["developer_instructions"])
	}
	if features, _ := parsed["features"].(map[string]any); features["shell_tool"] != false {
		t.Errorf("features = %v, want shell_tool = false", parsed["features"])
	}
}

func TestAdapterTransformDelegateRequiredFields(t *testing.T) {
	// Codex ignores an agent missing a description or instructions, so the
	// description falls back to the name and an empty body warns.
	got, warnings := transformDelegate(t, map[string]string{"name": "r"}, nil, "  \n")

	if description := parseAgent(t, got)["description"]; description != "r" {
		t.Errorf("description = %v, want the name as fallback", description)
	}
	if !containsSubstring(warnings, "body is empty") {
		t.Errorf("missing empty-body warning, warnings: %q", warnings)
	}
}

func TestAdapterTransformDirectoryDelegateStaysMarkdown(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypeDelegate, Name: "DELEGATE.md", IsDir: true},
		Metadata: map[string]string{
			"name":        "code-reviewer",
			"description": "Reviews code for quality",
			"tools":       "Read, Glob, Grep",
			"model":       "gpt-5",
		},
		Content: []byte("You are a code reviewer.\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\nname: code-reviewer\ndescription: Reviews code for quality\ntools: Read, Glob, Grep\nmodel: gpt-5\n---\n\nYou are a code reviewer.\n"
	if string(got) != want {
		t.Errorf("directory delegate transform mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func containsSubstring(items []string, sub string) bool {
	for _, item := range items {
		if strings.Contains(item, sub) {
			return true
		}
	}
	return false
}

func TestAdapterTransformSkill(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypeSkill},
		Metadata: map[string]string{
			"name":        "my-skill",
			"description": "A skill description",
			"tools":       "Read, Edit",
			"model":       "gpt-5",
		},
		Content: []byte("Skill instructions.\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	gotStr := string(got)
	if !strings.Contains(gotStr, "name: my-skill") {
		t.Fatalf("skill transform missing name field: %q", gotStr)
	}
	if !strings.Contains(gotStr, "description: A skill description") {
		t.Fatalf("skill transform missing description field: %q", gotStr)
	}
	if strings.Contains(gotStr, "tools:") {
		t.Fatalf("skill transform unexpectedly contains tools field: %q", gotStr)
	}
	if strings.Contains(gotStr, "model:") {
		t.Fatalf("skill transform unexpectedly contains model field: %q", gotStr)
	}
}

func TestAdapterTransformFilePassthrough(t *testing.T) {
	a := New("")
	content := []byte("plain file\n")
	asset := &config.ResolvedAsset{
		Asset:   config.Asset{Type: config.AssetTypeFile},
		Content: content,
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file transform mismatch: got %q, want %q", string(got), string(content))
	}
}

func TestAdapterSupportedConfigAssetTypes(t *testing.T) {
	a := New("")
	got := a.SupportedConfigAssetTypes()

	want := []string{config.AssetTypeHook, config.AssetTypeMCP}
	if len(got) != len(want) {
		t.Fatalf("SupportedConfigAssetTypes() = %v, want %v", got, want)
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("SupportedConfigAssetTypes() = %v, missing %q", got, w)
		}
	}
}

func TestAdapterInstallWarnings(t *testing.T) {
	a := New("")

	personaWarnings := a.InstallWarnings(&config.Asset{Type: config.AssetTypePersona, Name: "senior-swe.md"})
	if len(personaWarnings) != 1 {
		t.Fatalf("len(persona warnings) = %d, want 1", len(personaWarnings))
	}
	if !strings.Contains(personaWarnings[0], "installed as skill instead: skills/senior-swe/SKILL.md") {
		t.Errorf("persona warning should mention skill directory path, got: %q", personaWarnings[0])
	}

	if warnings := a.InstallWarnings(&config.Asset{Type: config.AssetTypeDelegate, Name: "d.md"}); len(warnings) != 0 {
		t.Errorf("single-file delegate warnings = %q, want none: it installs as a Codex agent", warnings)
	}

	dirWarnings := a.InstallWarnings(&config.Asset{Type: config.AssetTypeDelegate, Name: "d", IsDir: true})
	if len(dirWarnings) != 1 {
		t.Fatalf("len(directory delegate warnings) = %d, want 1", len(dirWarnings))
	}
	if !strings.Contains(dirWarnings[0], "passive reference at delegates/d") {
		t.Errorf("directory delegate warning = %q, want passive reference message", dirWarnings[0])
	}

	if warnings := a.InstallWarnings(&config.Asset{Type: config.AssetTypeSkill, Name: "s.md"}); len(warnings) != 0 {
		t.Errorf("skill warnings len = %d, want 0", len(warnings))
	}
}

func TestAdapterSettingsFunctions(t *testing.T) {
	a := New("")

	if got := a.ConfigFilePath(config.AssetTypeStatusLine, config.ScopeUser); got != "" {
		t.Errorf("ConfigFilePath(statusline, user) = %q, want empty string", got)
	}
	if got := a.SettingsKey(config.AssetTypeStatusLine); got != "" {
		t.Errorf("SettingsKey('statusline') = %q, want empty string", got)
	}
}

func TestAdapterMainFileName(t *testing.T) {
	a := New("")
	if got := a.MainFileName(config.AssetTypeSkill); got != "SKILL.md" {
		t.Errorf("MainFileName('skill') = %q, want 'SKILL.md'", got)
	}
}

func TestAdapterProvideVariables(t *testing.T) {
	a := New("")

	vars := a.ProvideVariables()

	if vars["target_name"] != "codex" {
		t.Errorf("target_name = %q, want 'codex'", vars["target_name"])
	}
	if vars["target_name_display"] != "Codex" {
		t.Errorf("target_name_display = %q, want 'Codex'", vars["target_name_display"])
	}
	if vars["target_agents_file"] != "AGENTS.md" {
		t.Errorf("target_agents_file = %q, want 'AGENTS.md'", vars["target_agents_file"])
	}
}

func TestAdapterTransformNativeMerged(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset:    config.Asset{Type: config.AssetTypeSkill},
		Metadata: map[string]string{"name": "my-skill", "description": "A skill"},
		Native: frontmatter.Native{
			"codex":  {"argument-hint": "<slot>"},
			"claude": {"disable-model-invocation": true},
		},
		Content: []byte("# Skill\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\nname: my-skill\ndescription: A skill\nargument-hint: <slot>\n---\n\n# Skill\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}
