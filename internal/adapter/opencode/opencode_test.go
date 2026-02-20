package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
)

func TestAdapterName(t *testing.T) {
	a := New("")
	if a.Name() != "opencode" {
		t.Errorf("Name() = %q, want 'opencode'", a.Name())
	}
}

func TestAdapterAssetMapping(t *testing.T) {
	a := New("")
	mapping := a.AssetMapping()

	expected := map[string]string{
		"persona":  "agents",
		"delegate": "agents",
		"skill":    "skills",
	}

	for k, v := range expected {
		if mapping[k] != v {
			t.Errorf("mapping[%q] = %q, want %q", k, mapping[k], v)
		}
	}

	// Verify both persona and delegate map to the same directory
	if mapping["persona"] != mapping["delegate"] {
		t.Errorf("persona and delegate should both map to 'agents', got %q and %q",
			mapping["persona"], mapping["delegate"])
	}
}

func TestAdapterTargetDir(t *testing.T) {
	// Test with explicit target dir
	a := New("/custom/path")
	if a.TargetDir() != "/custom/path" {
		t.Errorf("TargetDir() = %q, want '/custom/path'", a.TargetDir())
	}

	// Test XDG_CONFIG_HOME override
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")
	a2 := New("")
	if a2.TargetDir() != "/xdg/config/opencode" {
		t.Errorf("TargetDir() = %q, want '/xdg/config/opencode'", a2.TargetDir())
	}

	// Test default fallback (no XDG_CONFIG_HOME)
	t.Setenv("XDG_CONFIG_HOME", "")
	a3 := New("")
	home, err := os.UserHomeDir()
	if err == nil {
		expected := filepath.Join(home, ".config", "opencode")
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
			asset:    &config.Asset{Type: "persona", Name: "senior-swe.md"},
			expected: "agents/senior-swe.md",
		},
		{
			asset:    &config.Asset{Type: "delegate", Name: "code-reviewer.md"},
			expected: "agents/code-reviewer.md",
		},
		{
			asset:    &config.Asset{Type: "skill", Name: "my-skill"},
			expected: "skills/my-skill",
		},
		{
			// File type with empty dest goes to root
			asset:    &config.Asset{Type: "file", Name: "AGENTS.md", Dest: ""},
			expected: "AGENTS.md",
		},
		{
			// File type with directory dest (no extension)
			asset:    &config.Asset{Type: "file", Name: "config.json", Dest: "config"},
			expected: "config/config.json",
		},
		{
			// File type with filename dest (has extension) uses dest as full path
			asset:    &config.Asset{Type: "file", Name: "rules.md", Dest: "AGENTS.md"},
			expected: "AGENTS.md",
		},
		{
			// File type with path+filename dest
			asset:    &config.Asset{Type: "file", Name: "settings.yaml", Dest: "config/app.yaml"},
			expected: "config/app.yaml",
		},
		{
			// File type with trailing slash dest
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

	want := "---\ndescription: A senior software engineer persona\n---\n\n# Senior Software Engineer\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestAdapterTransformPersonaNoDescription(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypePersona},
		Metadata: map[string]string{
			"name": "Senior SWE",
			// No description - should fall back to name
		},
		Content: []byte("# Senior Software Engineer\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\ndescription: Senior SWE\n---\n\n# Senior Software Engineer\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestAdapterTransformPersonaNoMetadata(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset:    config.Asset{Type: config.AssetTypePersona},
		Metadata: map[string]string{},
		Content:  []byte("# Bare persona\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	// With no name or description, should skip frontmatter entirely
	want := "# Bare persona\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestAdapterTransformDelegate(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypeDelegate},
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

	want := "---\ndescription: Reviews code for quality\nmode: subagent\nmodel: gpt-5\n---\n\nYou are a code reviewer.\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}

	// Verify tools are NOT present (incompatible format between tai and OpenCode)
	if strings.Contains(string(got), "tools:") {
		t.Errorf("delegate transform should not contain tools field: %q", string(got))
	}
}

func TestAdapterTransformDelegateNoDescription(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypeDelegate},
		Metadata: map[string]string{
			"name": "code-reviewer",
		},
		Content: []byte("You are a code reviewer.\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\ndescription: code-reviewer\nmode: subagent\n---\n\nYou are a code reviewer.\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestAdapterTransformDelegateNoModel(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypeDelegate},
		Metadata: map[string]string{
			"name":        "helper",
			"description": "A helper agent",
		},
		Content: []byte("Help the user.\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\ndescription: A helper agent\nmode: subagent\n---\n\nHelp the user.\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestAdapterTransformDelegateNoMetadata(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset:    config.Asset{Type: config.AssetTypeDelegate},
		Metadata: map[string]string{},
		Content:  []byte("Bare delegate.\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	// Even with no description, mode: subagent is always emitted
	want := "---\nmode: subagent\n---\n\nBare delegate.\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
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

	want := "---\nname: my-skill\ndescription: A skill description\n---\n\nSkill instructions.\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}

	// Skills should not include tools or model (OpenCode uses minimal frontmatter)
	if strings.Contains(string(got), "tools:") {
		t.Errorf("skill transform should not contain tools: %q", string(got))
	}
	if strings.Contains(string(got), "model:") {
		t.Errorf("skill transform should not contain model: %q", string(got))
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
	types := a.SupportedConfigAssetTypes()

	if len(types) != 0 {
		t.Errorf("len(SupportedConfigAssetTypes()) = %d, want 0", len(types))
	}
}

func TestAdapterInstallWarnings(t *testing.T) {
	a := New("")

	// OpenCode consumes all these asset types natively - no warnings expected
	for _, assetType := range []string{
		config.AssetTypePersona,
		config.AssetTypeDelegate,
		config.AssetTypeSkill,
		config.AssetTypeFile,
	} {
		warnings := a.InstallWarnings(&config.Asset{Type: assetType, Name: "test.md"})
		if len(warnings) != 0 {
			t.Errorf("InstallWarnings(%q) returned %d warnings, want 0", assetType, len(warnings))
		}
	}
}

func TestAdapterSettingsFunctions(t *testing.T) {
	a := New("")

	if got := a.SettingsFilePath("user"); got != "" {
		t.Errorf("SettingsFilePath('user') = %q, want empty string", got)
	}
	if got := a.SettingsKey(config.AssetTypeStatusLine); got != "" {
		t.Errorf("SettingsKey('statusline') = %q, want empty string", got)
	}
	if _, err := a.TransformStatusLine(nil, nil); err == nil {
		t.Error("TransformStatusLine() error = nil, want error")
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

	if vars["target_name"] != "opencode" {
		t.Errorf("target_name = %q, want 'opencode'", vars["target_name"])
	}
	if vars["target_name_display"] != "OpenCode" {
		t.Errorf("target_name_display = %q, want 'OpenCode'", vars["target_name_display"])
	}
	if vars["target_agents_file"] != "AGENTS.md" {
		t.Errorf("target_agents_file = %q, want 'AGENTS.md'", vars["target_agents_file"])
	}
}
