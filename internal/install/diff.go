package install

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// GenerateDiff returns a unified diff string for a conflict.
// For directory conflicts, returns a placeholder since diffing directory trees is complex.
// For config asset conflicts (nil content), returns a placeholder.
func GenerateDiff(conflict ConflictDetail) string {
	if conflict.IsDir {
		return "(directory conflict - inspect manually or use --force to replace)"
	}

	// Config asset conflicts don't have content to diff
	if conflict.ExistingContent == nil && conflict.ResolvedContent == nil {
		return "(config conflict - diff not available)"
	}

	// Handle nil content gracefully
	existingLines := splitLines(conflict.ExistingContent)
	resolvedLines := splitLines(conflict.ResolvedContent)

	diff := difflib.UnifiedDiff{
		A:        existingLines,
		B:        resolvedLines,
		FromFile: "a/" + conflict.Path,
		ToFile:   "b/" + conflict.Path,
		Context:  3,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return "(error generating diff: " + err.Error() + ")"
	}

	if result == "" {
		return "(no differences)"
	}

	return result
}

// splitLines converts byte content to a slice of lines for difflib.
// Each line includes its trailing newline if present.
func splitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}

	text := string(content)
	var lines []string

	for len(text) > 0 {
		idx := strings.Index(text, "\n")
		if idx == -1 {
			// Last line without trailing newline
			lines = append(lines, text)
			break
		}
		lines = append(lines, text[:idx+1])
		text = text[idx+1:]
	}

	return lines
}
