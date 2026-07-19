package patch

import (
	"bytes"
	"fmt"
	"strings"
)

// ApplyError reports an edit that could not be applied unambiguously.
type ApplyError struct {
	Patch     *Patch
	EditIndex int // 0-based; render as EditIndex+1
	Find      string
	Matches   int   // 0 = not found, >=2 = ambiguous
	Lines     []int // 1-based line numbers of each match, when ambiguous
	Hint      string
}

func (e *ApplyError) Error() string {
	if e.Matches == 0 {
		return fmt.Sprintf("patch %q edit #%d: find text not found", e.Patch.Name, e.EditIndex+1)
	}
	return fmt.Sprintf("patch %q edit #%d: ambiguous - matched %d locations (lines %s)",
		e.Patch.Name, e.EditIndex+1, e.Matches, joinInts(e.Lines))
}

// Apply runs every edit of p against content, in order. Each edit sees the
// result of the previous one, so edits can build on each other.
func Apply(content []byte, p *Patch) ([]byte, *ApplyError) {
	out := content
	for i, edit := range p.Edits {
		find := []byte(edit.Find)

		// Count is non-overlapping, so "aa" in "aaa" counts once. Documented as
		// a known quirk rather than worked around: overlapping finds are a sign
		// the find text needs more surrounding context anyway.
		n := bytes.Count(out, find)
		if n != 1 {
			return nil, &ApplyError{
				Patch:     p,
				EditIndex: i,
				Find:      edit.Find,
				Matches:   n,
				Lines:     matchLines(out, find),
				Hint:      hintFor(out, edit.Find, n),
			}
		}
		out = bytes.Replace(out, find, []byte(edit.Replace), 1)
	}
	return out, nil
}

// ApplyAll runs several patches over one file, in the given order, and returns
// the names of those applied.
func ApplyAll(content []byte, patches []*Patch) ([]byte, []string, *ApplyError) {
	if len(patches) == 0 {
		return content, nil, nil
	}
	out := content
	applied := make([]string, 0, len(patches))
	for _, p := range patches {
		next, err := Apply(out, p)
		if err != nil {
			return nil, nil, err
		}
		out = next
		applied = append(applied, p.Name)
	}
	return out, applied, nil
}

// matchLines returns the 1-based line number of each match of find.
func matchLines(content, find []byte) []int {
	var lines []int
	offset := 0
	for {
		idx := bytes.Index(content[offset:], find)
		if idx < 0 {
			return lines
		}
		abs := offset + idx
		lines = append(lines, bytes.Count(content[:abs], []byte("\n"))+1)
		offset = abs + len(find)
	}
}

// hintFor diagnoses the two mistakes that account for most failed finds:
// YAML block scalars appending a newline, and mismatched line endings. Both
// produce a find that looks byte-identical to the eye, so without a hint the
// user is left staring at text that "obviously" matches.
func hintFor(content []byte, find string, matches int) string {
	if matches != 0 {
		return "add more surrounding context to 'find' so it matches exactly one location"
	}

	if trimmed := strings.TrimSuffix(find, "\n"); trimmed != find && trimmed != "" {
		if bytes.Contains(content, []byte(trimmed)) {
			return "the text matches without its trailing newline - YAML's `find: |` appends one; use `find: |-` instead"
		}
	}

	// Try both directions so we catch a CRLF find against LF content and vice versa.
	if swapped := swapLineEndings(find); swapped != find && swapped != "" {
		if bytes.Contains(content, []byte(swapped)) {
			return "the text matches with different line endings - the file and the patch disagree on CRLF vs LF"
		}
	}

	return ""
}

func swapLineEndings(s string) string {
	if strings.Contains(s, "\r\n") {
		return strings.ReplaceAll(s, "\r\n", "\n")
	}
	return strings.ReplaceAll(s, "\n", "\r\n")
}

func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, ", ")
}
