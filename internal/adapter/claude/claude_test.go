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
		"personas":  "output-styles",
		"delegates": "agents",
		"skills":    "skills",
		"docs":      "",
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
			asset:    &config.Asset{Type: "personas", Name: "senior-swe.md"},
			expected: "output-styles/senior-swe.md",
		},
		{
			asset:    &config.Asset{Type: "delegates", Name: "code-reviewer.md"},
			expected: "agents/code-reviewer.md",
		},
		{
			asset:    &config.Asset{Type: "skills", Name: "my-skill"},
			expected: "skills/my-skill",
		},
		{
			asset:    &config.Asset{Type: "docs", Name: "CLAUDE.md"},
			expected: "CLAUDE.md",
		},
		{
			asset:    &config.Asset{Type: "docs", Name: "RFC2119.md"},
			expected: "RFC2119.md",
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
