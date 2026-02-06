package install

import (
	"strings"
	"testing"
)

func TestGenerateDiffChangedContent(t *testing.T) {
	conflict := ConflictDetail{
		Path:            "output-styles/test.md",
		FullPath:        "/target/output-styles/test.md",
		Reason:          "file modified since last install",
		ExistingContent: []byte("line1\nline2\nline3\n"),
		ResolvedContent: []byte("line1\nmodified\nline3\n"),
	}

	diff := GenerateDiff(conflict)

	if !strings.Contains(diff, "--- a/output-styles/test.md") {
		t.Error("diff should contain '--- a/output-styles/test.md' header")
	}
	if !strings.Contains(diff, "+++ b/output-styles/test.md") {
		t.Error("diff should contain '+++ b/output-styles/test.md' header")
	}
	if !strings.Contains(diff, "-line2") {
		t.Error("diff should show removed line")
	}
	if !strings.Contains(diff, "+modified") {
		t.Error("diff should show added line")
	}
}

func TestGenerateDiffIdenticalContent(t *testing.T) {
	conflict := ConflictDetail{
		Path:            "output-styles/test.md",
		ExistingContent: []byte("same\ncontent\n"),
		ResolvedContent: []byte("same\ncontent\n"),
	}

	diff := GenerateDiff(conflict)

	if diff != "(no differences)" {
		t.Errorf("expected '(no differences)', got %q", diff)
	}
}

func TestGenerateDiffDirectoryConflict(t *testing.T) {
	conflict := ConflictDetail{
		Path:   "skills/my-skill",
		IsDir:  true,
		Reason: "unmanaged directory exists",
	}

	diff := GenerateDiff(conflict)

	if diff != "(directory conflict - inspect manually or use --force to replace)" {
		t.Errorf("expected directory placeholder, got %q", diff)
	}
}

func TestGenerateDiffEmptyExisting(t *testing.T) {
	conflict := ConflictDetail{
		Path:            "output-styles/new.md",
		ExistingContent: nil,
		ResolvedContent: []byte("new content\n"),
	}

	diff := GenerateDiff(conflict)

	if !strings.Contains(diff, "+new content") {
		t.Errorf("diff should show added content, got %q", diff)
	}
}

func TestGenerateDiffEmptyResolved(t *testing.T) {
	conflict := ConflictDetail{
		Path:            "output-styles/old.md",
		ExistingContent: []byte("old content\n"),
		ResolvedContent: nil,
	}

	diff := GenerateDiff(conflict)

	if !strings.Contains(diff, "-old content") {
		t.Errorf("diff should show removed content, got %q", diff)
	}
}

func TestGenerateDiffBothEmpty(t *testing.T) {
	// When both contents are nil, this is treated as a config conflict
	conflict := ConflictDetail{
		Path:            "[statusLine]",
		ExistingContent: nil,
		ResolvedContent: nil,
	}

	diff := GenerateDiff(conflict)

	if diff != "(config conflict - diff not available)" {
		t.Errorf("expected '(config conflict - diff not available)', got %q", diff)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []string
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single line with newline",
			input:    []byte("hello\n"),
			expected: []string{"hello\n"},
		},
		{
			name:     "single line without newline",
			input:    []byte("hello"),
			expected: []string{"hello"},
		},
		{
			name:     "multiple lines",
			input:    []byte("line1\nline2\nline3\n"),
			expected: []string{"line1\n", "line2\n", "line3\n"},
		},
		{
			name:     "multiple lines no trailing newline",
			input:    []byte("line1\nline2\nline3"),
			expected: []string{"line1\n", "line2\n", "line3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("len(result) = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i, line := range result {
				if line != tt.expected[i] {
					t.Errorf("line[%d] = %q, want %q", i, line, tt.expected[i])
				}
			}
		})
	}
}
