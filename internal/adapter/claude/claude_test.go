package claude

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
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
			// File type with directory dest (no extension) goes to subdirectory
			asset:    &config.Asset{Type: "file", Name: "config.json", Dest: "config"},
			expected: "config/config.json",
		},
		{
			// File type with filename dest (has extension) uses dest as full path
			asset:    &config.Asset{Type: "file", Name: "AGENTS.md", Dest: "CLAUDE.md"},
			expected: "CLAUDE.md",
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

	// Unknown types use type/name format
	expected := "unknown/test.md"
	if result != expected {
		t.Errorf("MapAssetPath for unknown type = %q, want %q", result, expected)
	}
}

func TestAdapterSupportedConfigAssetTypes(t *testing.T) {
	a := New("")
	got := a.SupportedConfigAssetTypes()

	want := []string{config.AssetTypeStatusLine, config.AssetTypeHook, config.AssetTypeMCP, config.AssetTypeSettings, config.AssetTypeApproval}
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

func TestAdapterConfigFilePath(t *testing.T) {
	a := New("/custom/path")

	userPath := a.ConfigFilePath(config.AssetTypeStatusLine, config.ScopeUser)
	if userPath != "/custom/path/settings.json" {
		t.Errorf("ConfigFilePath(statusline, user) = %q, want '/custom/path/settings.json'", userPath)
	}

	// Project and local scopes resolve against the working directory, and are
	// returned absolute so nothing downstream has to re-resolve them.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	projectPath := a.ConfigFilePath(config.AssetTypeStatusLine, config.ScopeProject)
	if want := filepath.Join(wd, ".claude", "settings.json"); projectPath != want {
		t.Errorf("ConfigFilePath(statusline, project) = %q, want %q", projectPath, want)
	}

	localPath := a.ConfigFilePath(config.AssetTypeStatusLine, config.ScopeLocal)
	if want := filepath.Join(wd, ".claude", "settings.local.json"); localPath != want {
		t.Errorf("ConfigFilePath(statusline, local) = %q, want %q", localPath, want)
	}

	if got := a.ConfigFilePath("nonexistent-type", config.ScopeUser); got != "" {
		t.Errorf("ConfigFilePath(nonexistent-type, user) = %q, want empty string", got)
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

func TestAdapterProvideVariables(t *testing.T) {
	a := New("")

	vars := a.ProvideVariables()

	if vars["target_name"] != "claude" {
		t.Errorf("target_name = %q, want 'claude'", vars["target_name"])
	}
	if vars["target_name_display"] != "Claude" {
		t.Errorf("target_name_display = %q, want 'Claude'", vars["target_name_display"])
	}
	if vars["target_agents_file"] != "CLAUDE.md" {
		t.Errorf("target_agents_file = %q, want 'CLAUDE.md'", vars["target_agents_file"])
	}
}

func TestAdapterTransformNativeMerged(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypeSkill},
		Metadata: map[string]string{
			"name":        "parallel-work",
			"description": "Parallel worktree workflow",
		},
		Native: frontmatter.Native{
			"claude": {"disable-model-invocation": true},
			"codex":  {"argument-hint": "<slot>"},
		},
		Content: []byte("# Parallel Work\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\nname: parallel-work\ndescription: Parallel worktree workflow\n" +
		"disable-model-invocation: true\n---\n\n# Parallel Work\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestAdapterTransformNativeOverridesTaiField(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset: config.Asset{Type: config.AssetTypeDelegate},
		Metadata: map[string]string{
			"name":        "reviewer",
			"description": "tai's description",
			"tools":       "Read, Grep",
		},
		Native:  frontmatter.Native{"claude": {"description": "Claude's own description"}},
		Content: []byte("# Reviewer\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	// Native wins, and keeps the position tai gave the field.
	want := "---\nname: reviewer\ndescription: Claude's own description\ntools: Read, Grep\n---\n\n# Reviewer\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestAdapterTransformPersonaNative(t *testing.T) {
	a := New("")
	asset := &config.ResolvedAsset{
		Asset:    config.Asset{Type: config.AssetTypePersona},
		Metadata: map[string]string{"name": "Senior SWE"},
		Native:   frontmatter.Native{"claude": {"model": "opus"}},
		Content:  []byte("# Senior SWE\n"),
	}

	got, err := a.TransformContent(asset)
	if err != nil {
		t.Fatalf("TransformContent failed: %v", err)
	}

	want := "---\nname: Senior SWE\nmodel: opus\n---\n\n# Senior SWE\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", string(got), want)
	}
}
