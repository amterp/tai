package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
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
		"persona":  "personas",
		"delegate": "delegates",
		"skill":    "skills",
	}

	for k, v := range expected {
		if mapping[k] != v {
			t.Errorf("mapping[%q] = %q, want %q", k, mapping[k], v)
		}
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
			asset:    &config.Asset{Type: "persona", Name: "senior-swe.md"},
			expected: "personas/senior-swe.md",
		},
		{
			asset:    &config.Asset{Type: "delegate", Name: "code-reviewer.md"},
			expected: "delegates/code-reviewer.md",
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

	want := "---\nname: code-reviewer\ndescription: Reviews code for quality\ntools: Read, Glob, Grep\nmodel: gpt-5\n---\n\nYou are a code reviewer.\n"
	if string(got) != want {
		t.Errorf("delegate transform mismatch:\n got: %q\nwant: %q", string(got), want)
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
	types := a.SupportedConfigAssetTypes()

	if len(types) != 0 {
		t.Errorf("len(SupportedConfigAssetTypes()) = %d, want 0", len(types))
	}
}

func TestAdapterInstallWarnings(t *testing.T) {
	a := New("")

	personaWarnings := a.InstallWarnings(&config.Asset{Type: config.AssetTypePersona, Name: "p.md"})
	if len(personaWarnings) != 1 {
		t.Fatalf("len(persona warnings) = %d, want 1", len(personaWarnings))
	}
	if !strings.Contains(personaWarnings[0], "passive reference") {
		t.Errorf("persona warning = %q, want passive reference message", personaWarnings[0])
	}

	delegateWarnings := a.InstallWarnings(&config.Asset{Type: config.AssetTypeDelegate, Name: "d.md"})
	if len(delegateWarnings) != 1 {
		t.Fatalf("len(delegate warnings) = %d, want 1", len(delegateWarnings))
	}
	if !strings.Contains(delegateWarnings[0], "passive reference") {
		t.Errorf("delegate warning = %q, want passive reference message", delegateWarnings[0])
	}

	if warnings := a.InstallWarnings(&config.Asset{Type: config.AssetTypeSkill, Name: "s.md"}); len(warnings) != 0 {
		t.Errorf("skill warnings len = %d, want 0", len(warnings))
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
