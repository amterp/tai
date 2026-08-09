package gemini

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
)

func TestAdapterName(t *testing.T) {
	a := New("")
	if a.Name() != "gemini" {
		t.Errorf("Name() = %q, want 'gemini'", a.Name())
	}
}

func TestAdapterAssetMapping(t *testing.T) {
	a := New("")
	mapping := a.AssetMapping()

	expected := map[string]string{
		"persona": "skills",
		"skill":   "skills",
	}

	for k, v := range expected {
		if mapping[k] != v {
			t.Errorf("mapping[%q] = %q, want %q", k, mapping[k], v)
		}
	}

	// Verify personas and skills both map to skills/
	if mapping["persona"] != mapping["skill"] {
		t.Errorf("persona and skill should both map to 'skills', got %q and %q",
			mapping["persona"], mapping["skill"])
	}

	// Verify delegate is NOT in the mapping (handled by UnsupportedFileAssetTypes)
	if _, ok := mapping["delegate"]; ok {
		t.Error("delegate should not be in asset mapping")
	}
}

func TestAdapterTargetDir(t *testing.T) {
	// Test with explicit target dir
	a := New("/custom/path")
	if a.TargetDir() != "/custom/path" {
		t.Errorf("TargetDir() = %q, want '/custom/path'", a.TargetDir())
	}

	// Test GEMINI_HOME override
	t.Setenv("GEMINI_HOME", "/gemini/home")
	a2 := New("")
	if a2.TargetDir() != "/gemini/home" {
		t.Errorf("TargetDir() = %q, want '/gemini/home'", a2.TargetDir())
	}

	// Test default fallback (no GEMINI_HOME)
	t.Setenv("GEMINI_HOME", "")
	a3 := New("")
	home, err := os.UserHomeDir()
	if err == nil {
		expected := filepath.Join(home, ".gemini")
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
			// Single-file persona wrapped in directory for Gemini
			asset:    &config.Asset{Type: "persona", Name: "senior-swe.md"},
			expected: "skills/senior-swe/SKILL.md",
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
			asset:    &config.Asset{Type: "file", Name: "GEMINI.md", Dest: ""},
			expected: "GEMINI.md",
		},
		{
			// File type with directory dest (no extension)
			asset:    &config.Asset{Type: "file", Name: "config.json", Dest: "config"},
			expected: "config/config.json",
		},
		{
			// File type with filename dest (has extension) uses dest as full path
			asset:    &config.Asset{Type: "file", Name: "rules.md", Dest: "GEMINI.md"},
			expected: "GEMINI.md",
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

	// Personas are transformed as skills (name + description)
	want := "---\nname: Senior SWE\ndescription: A senior software engineer persona\n---\n\n# Senior Software Engineer\n"
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
		},
		Content: []byte("# Senior Software Engineer\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\nname: Senior SWE\n---\n\n# Senior Software Engineer\n"
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
			"model":       "gemini-2.5-pro",
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

	// Skills should not include tools or model (Gemini skills use minimal frontmatter)
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

	if len(types) != 1 || types[0] != config.AssetTypeHook {
		t.Errorf("SupportedConfigAssetTypes() = %v, want [hook]", types)
	}
}

func TestAdapterUnsupportedFileAssetTypes(t *testing.T) {
	a := New("")
	types := a.UnsupportedFileAssetTypes()

	if len(types) != 1 {
		t.Fatalf("len(UnsupportedFileAssetTypes()) = %d, want 1", len(types))
	}
	if types[0] != config.AssetTypeDelegate {
		t.Errorf("UnsupportedFileAssetTypes()[0] = %q, want %q", types[0], config.AssetTypeDelegate)
	}
}

func TestAdapterInstallWarnings(t *testing.T) {
	a := New("")

	// Persona should warn about mapping to skill
	warnings := a.InstallWarnings(&config.Asset{Type: config.AssetTypePersona, Name: "senior-swe.md"})
	if len(warnings) != 1 {
		t.Fatalf("InstallWarnings(persona) returned %d warnings, want 1", len(warnings))
	}
	if !strings.Contains(warnings[0], "installed as skill instead: skills/senior-swe/SKILL.md") {
		t.Errorf("persona warning should mention skill directory path, got: %q", warnings[0])
	}

	// Other types should not warn
	for _, assetType := range []string{
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

	if vars["target_name"] != "gemini" {
		t.Errorf("target_name = %q, want 'gemini'", vars["target_name"])
	}
	if vars["target_name_display"] != "Gemini" {
		t.Errorf("target_name_display = %q, want 'Gemini'", vars["target_name_display"])
	}
	if vars["target_agents_file"] != "GEMINI.md" {
		t.Errorf("target_agents_file = %q, want 'GEMINI.md'", vars["target_agents_file"])
	}
}
