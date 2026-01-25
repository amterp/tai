package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/config"
)

func TestAdapterName(t *testing.T) {
	a := New("")
	if a.Name() != "claude" {
		t.Errorf("Name() = %q, want 'claude'", a.Name())
	}
}

func TestAdapterAssetMapping(t *testing.T) {
	a := New("")
	mapping := a.AssetMapping()

	expected := map[string]string{
		"persona":  "output-styles",
		"delegate": "agents",
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

	// Test default (should be ~/.claude)
	a2 := New("")
	home, err := os.UserHomeDir()
	if err == nil {
		expected := filepath.Join(home, ".claude")
		if a2.TargetDir() != expected {
			t.Errorf("TargetDir() = %q, want %q", a2.TargetDir(), expected)
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
			expected: "output-styles/senior-swe.md",
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
			asset:    &config.Asset{Type: "file", Name: "CLAUDE.md", Dest: ""},
			expected: "CLAUDE.md",
		},
		{
			// File type with empty dest goes to root
			asset:    &config.Asset{Type: "file", Name: "RFC2119.md", Dest: ""},
			expected: "RFC2119.md",
		},
		{
			// File type with dest goes to subdirectory
			asset:    &config.Asset{Type: "file", Name: "config.json", Dest: "config"},
			expected: "config/config.json",
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

	// Unknown types use type/name format
	expected := "unknown/test.md"
	if result != expected {
		t.Errorf("MapAssetPath for unknown type = %q, want %q", result, expected)
	}
}

func TestAdapterSupportedConfigAssetTypes(t *testing.T) {
	a := New("")
	types := a.SupportedConfigAssetTypes()

	if len(types) != 1 {
		t.Errorf("len(SupportedConfigAssetTypes()) = %d, want 1", len(types))
	}
	if types[0] != "statusline" {
		t.Errorf("SupportedConfigAssetTypes()[0] = %q, want 'statusline'", types[0])
	}
}

func TestAdapterSettingsFilePath(t *testing.T) {
	a := New("/custom/path")

	userPath := a.SettingsFilePath("user")
	if userPath != "/custom/path/settings.json" {
		t.Errorf("SettingsFilePath('user') = %q, want '/custom/path/settings.json'", userPath)
	}

	projectPath := a.SettingsFilePath("project")
	if projectPath != ".claude/settings.json" {
		t.Errorf("SettingsFilePath('project') = %q, want '.claude/settings.json'", projectPath)
	}
}

func TestAdapterTransformStatusLineDetailed(t *testing.T) {
	a := New("")

	cfg := &config.StatusLineConfig{
		Level: "detailed",
		Show:  []string{"cost", "model", "tokens"},
	}

	result, err := a.TransformStatusLine(cfg, nil)
	if err != nil {
		t.Fatalf("TransformStatusLine failed: %v", err)
	}

	if result["style"] != "detailed" {
		t.Errorf("style = %v, want 'detailed'", result["style"])
	}
	if result["showCost"] != true {
		t.Errorf("showCost = %v, want true", result["showCost"])
	}
	if result["showModel"] != true {
		t.Errorf("showModel = %v, want true", result["showModel"])
	}
	if result["showTokens"] != true {
		t.Errorf("showTokens = %v, want true", result["showTokens"])
	}
	if result["showLatency"] != false {
		t.Errorf("showLatency = %v, want false", result["showLatency"])
	}
}

func TestAdapterTransformStatusLineMinimal(t *testing.T) {
	a := New("")

	cfg := &config.StatusLineConfig{
		Level: "minimal",
		Show:  []string{"model"},
	}

	result, err := a.TransformStatusLine(cfg, nil)
	if err != nil {
		t.Fatalf("TransformStatusLine failed: %v", err)
	}

	if result["style"] != "minimal" {
		t.Errorf("style = %v, want 'minimal'", result["style"])
	}
	if result["showModel"] != true {
		t.Errorf("showModel = %v, want true", result["showModel"])
	}
	if result["showCost"] != false {
		t.Errorf("showCost = %v, want false", result["showCost"])
	}
}

func TestAdapterTransformStatusLineHidden(t *testing.T) {
	a := New("")

	cfg := &config.StatusLineConfig{
		Level: "hidden",
	}

	result, err := a.TransformStatusLine(cfg, nil)
	if err != nil {
		t.Fatalf("TransformStatusLine failed: %v", err)
	}

	if result["style"] != "hidden" {
		t.Errorf("style = %v, want 'hidden'", result["style"])
	}
}

func TestAdapterTransformStatusLineCustom(t *testing.T) {
	a := New("")

	padding := 2
	cfg := &config.StatusLineConfig{
		Level:   "custom",
		Command: "~/.scripts/statusline.sh",
		Padding: &padding,
	}

	result, err := a.TransformStatusLine(cfg, nil)
	if err != nil {
		t.Fatalf("TransformStatusLine failed: %v", err)
	}

	if result["type"] != "command" {
		t.Errorf("type = %v, want 'command'", result["type"])
	}
	if result["command"] != "~/.scripts/statusline.sh" {
		t.Errorf("command = %v, want '~/.scripts/statusline.sh'", result["command"])
	}
	if result["padding"] != 2 {
		t.Errorf("padding = %v, want 2", result["padding"])
	}
}

func TestAdapterTransformStatusLineNativePassthrough(t *testing.T) {
	a := New("")

	// JSON native format
	native := []byte(`{"style": "detailed", "showCost": true}`)

	result, err := a.TransformStatusLine(nil, native)
	if err != nil {
		t.Fatalf("TransformStatusLine failed: %v", err)
	}

	if result["style"] != "detailed" {
		t.Errorf("style = %v, want 'detailed'", result["style"])
	}
	if result["showCost"] != true {
		t.Errorf("showCost = %v, want true", result["showCost"])
	}
}
