package install

import (
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/frontmatter"
)

func TestFrontmatterWarningsUnknownField(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata: map[string]string{
			"name":                     "parallel-work",
			"description":              "A skill",
			"disable-model-invocation": "true",
		},
	}

	warnings := frontmatterWarnings(parsed, "skill", "parallel-work", "claude", nil)

	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	for _, want := range []string{"disable-model-invocation", "claude", "native.claude"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("warning missing %q: %s", want, warnings[0])
		}
	}
}

func TestFrontmatterWarningsIgnoresTaiFields(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata: map[string]string{
			"name":        "my-skill",
			"description": "A skill",
			"tools":       "Read, Grep",
			"model":       "opus",
		},
		Native: frontmatter.Native{"claude": {"effort": "high"}},
	}

	warnings := frontmatterWarnings(parsed, "skill", "my-skill", "claude", []string{"claude", "codex"})

	if len(warnings) != 0 {
		t.Errorf("got warnings for tai's own fields: %v", warnings)
	}
}

func TestFrontmatterWarningsUnknownFieldsAreSorted(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata: map[string]string{"name": "x", "zebra": "1", "alpha": "2"},
	}

	warnings := frontmatterWarnings(parsed, "skill", "x", "claude", nil)

	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want 2: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "alpha") || !strings.Contains(warnings[1], "zebra") {
		t.Errorf("warnings out of order: %v", warnings)
	}
}

func TestFrontmatterWarningsMisspelledNativeTarget(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata: map[string]string{"name": "my-skill"},
		Native:   frontmatter.Native{"cluade": {"effort": "high"}},
	}

	warnings := frontmatterWarnings(parsed, "skill", "my-skill", "claude", []string{"codex", "claude"})

	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	for _, want := range []string{"cluade", "claude, codex"} {
		if !strings.Contains(warnings[0], want) {
			t.Errorf("warning missing %q: %s", want, warnings[0])
		}
	}
}

// A nested unknown field is dropped like any other, so it warns like one.
func TestFrontmatterWarningsStructuredField(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata:   map[string]string{"name": "my-skill"},
		Structured: map[string]any{"metadata": map[string]any{"author": "x"}},
	}

	warnings := frontmatterWarnings(parsed, "skill", "my-skill", "claude", nil)

	if len(warnings) != 1 || !strings.Contains(warnings[0], `"metadata"`) {
		t.Fatalf("warnings = %v, want one naming the dropped metadata field", warnings)
	}
}

// A tai field written as a list is not installed as "[Read Grep]"; it is
// dropped, and says so.
func TestFrontmatterWarningsStructuredTaiField(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata:   map[string]string{"name": "my-skill"},
		Structured: map[string]any{"tools": []any{"Read", "Grep"}},
	}

	warnings := frontmatterWarnings(parsed, "skill", "my-skill", "claude", nil)

	if len(warnings) != 1 || !strings.Contains(warnings[0], `"tools"`) || !strings.Contains(warnings[0], "single value") {
		t.Fatalf("warnings = %v, want one saying tools takes a single value", warnings)
	}
}

// A removed target is not a typo, so the warning must not suggest checking
// the spelling.
func TestFrontmatterWarningsRemovedNativeTarget(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata: map[string]string{"name": "my-skill"},
		Native:   frontmatter.Native{"gemini": {"effort": "high"}},
	}

	warnings := frontmatterWarnings(parsed, "skill", "my-skill", "claude", []string{"codex", "claude"})

	if len(warnings) != 1 || !strings.Contains(warnings[0], "no longer supports") {
		t.Fatalf("warnings = %v, want one saying gemini is no longer supported", warnings)
	}
}

// With no adapters registered tai cannot tell a misspelled target from one it
// simply has not loaded, so it says nothing rather than crying wolf.
func TestFrontmatterWarningsNoKnownTargets(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata: map[string]string{"name": "my-skill"},
		Native:   frontmatter.Native{"cluade": {"effort": "high"}},
	}

	warnings := frontmatterWarnings(parsed, "skill", "my-skill", "claude", nil)

	if len(warnings) != 0 {
		t.Errorf("got warnings with no known targets: %v", warnings)
	}
}

func TestFrontmatterWarningsRetiredField(t *testing.T) {
	parsed := &frontmatter.Parsed{
		Metadata: map[string]string{"name": "my-skill", "tai-schema-version": "1"},
	}

	warnings := frontmatterWarnings(parsed, "skill", "my-skill", "claude", nil)

	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	// The generic advice would send the reader to native, which is wrong here.
	if strings.Contains(warnings[0], "native") {
		t.Errorf("retired field warning points at native: %s", warnings[0])
	}
	if !strings.Contains(warnings[0], "tai.yaml") {
		t.Errorf("warning does not say what replaced it: %s", warnings[0])
	}
}

func TestFileDestWarning(t *testing.T) {
	tests := []struct {
		name      string
		assetType string
		assetName string
		dest      string
		wantWarn  bool
	}{
		{"extensioned source into dot-name", "file", "plugin.json", "skills/tool-lines/.claude-plugin", true},
		{"dot-name at target root", "file", "plugin.json", ".claude-plugin", true},
		{"trailing slash makes it a directory", "file", "plugin.json", "skills/tool-lines/.claude-plugin/", false},
		{"dotfile source keeps its name", "file", ".gitignore", ".gitignore", false},
		{"stem rename to dotfile", "file", "gitignore.txt", ".gitignore", false},
		{"stem rename in subdirectory", "file", "envrc.sh", "project/.envrc", false},
		{"extensionless source", "file", "plugin", ".claude-plugin", false},
		{"ordinary file path", "file", "AGENTS.md", "CLAUDE.md", false},
		{"dot-name with its own extension", "file", "plugin.json", ".config.json", false},
		{"empty dest", "file", "plugin.json", "", false},
		{"non-file asset", "skill", "plugin.json", ".claude-plugin", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset := &config.Asset{Type: tt.assetType, Name: tt.assetName, Dest: tt.dest}
			got := fileDestWarning(asset, tt.dest)
			if (got != "") != tt.wantWarn {
				t.Errorf("fileDestWarning() = %q, want warning: %v", got, tt.wantWarn)
			}
		})
	}
}

// The fix the warning suggests has to be the dest as written, since that is
// the text the user edits; the resolved dest is what describes the outcome.
func TestFileDestWarningQuotesWrittenDestInFix(t *testing.T) {
	asset := &config.Asset{Type: "file", Name: "plugin.json", Dest: "{{ target_name }}/.claude-plugin"}

	got := fileDestWarning(asset, "claude/.claude-plugin")

	for _, want := range []string{`"claude/.claude-plugin"`, `"{{ target_name }}/.claude-plugin/"`} {
		if !strings.Contains(got, want) {
			t.Errorf("warning missing %s: %s", want, got)
		}
	}
}
