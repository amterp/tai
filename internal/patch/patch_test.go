package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkPatch(name string, edits ...Edit) *Patch {
	return &Patch{
		Version: 1, File: "skills/rad/SKILL.md", Edits: edits,
		Name: name, SourceName: "upstream",
	}
}

func TestApplyReplacesSingleMatch(t *testing.T) {
	content := []byte("line one\nAlways install with npm.\nline three\n")
	p := mkPatch("rad", Edit{Find: "Always install with npm.", Replace: "Always install with pnpm."})

	got, err := Apply(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line one\nAlways install with pnpm.\nline three\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyEmptyReplaceDeletes(t *testing.T) {
	content := []byte("keep\nDELETE ME\nkeep\n")
	p := mkPatch("del", Edit{Find: "DELETE ME\n", Replace: ""})

	got, err := Apply(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "keep\nkeep\n" {
		t.Errorf("got %q, want %q", got, "keep\nkeep\n")
	}
}

func TestApplyNotFound(t *testing.T) {
	content := []byte("nothing relevant here\n")
	p := mkPatch("rad", Edit{Find: "missing text", Replace: "x"})

	_, err := Apply(content, p)
	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Matches != 0 {
		t.Errorf("Matches = %d, want 0", err.Matches)
	}
	if err.EditIndex != 0 {
		t.Errorf("EditIndex = %d, want 0", err.EditIndex)
	}
	if !strings.Contains(err.Error(), "find text not found") {
		t.Errorf("Error() = %q, want it to mention 'find text not found'", err.Error())
	}
}

func TestApplyAmbiguousReportsLines(t *testing.T) {
	content := []byte("a\ndup\nb\nc\ndup\nd\ndup\n")
	p := mkPatch("rad", Edit{Find: "dup", Replace: "x"})

	_, err := Apply(content, p)
	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Matches != 3 {
		t.Errorf("Matches = %d, want 3", err.Matches)
	}
	want := []int{2, 5, 7}
	if len(err.Lines) != len(want) {
		t.Fatalf("Lines = %v, want %v", err.Lines, want)
	}
	for i, n := range want {
		if err.Lines[i] != n {
			t.Fatalf("Lines = %v, want %v", err.Lines, want)
		}
	}
	if !strings.Contains(err.Error(), "lines 2, 5, 7") {
		t.Errorf("Error() = %q, want it to list the match lines", err.Error())
	}
	if !strings.Contains(err.Hint, "surrounding context") {
		t.Errorf("Hint = %q, want a context hint", err.Hint)
	}
}

func TestApplySequentialEditsSeePriorResult(t *testing.T) {
	content := []byte("alpha beta\n")
	p := mkPatch("seq",
		Edit{Find: "alpha", Replace: "gamma"},
		Edit{Find: "gamma beta", Replace: "done"},
	)

	got, err := Apply(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "done\n" {
		t.Errorf("got %q, want %q", got, "done\n")
	}
}

func TestApplyFailingEditLeavesNoPartialResult(t *testing.T) {
	content := []byte("alpha beta\n")
	p := mkPatch("seq",
		Edit{Find: "alpha", Replace: "gamma"},
		Edit{Find: "nonexistent", Replace: "x"},
	)

	got, err := Apply(content, p)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got != nil {
		t.Errorf("got = %q, want nil on failure", got)
	}
	if err.EditIndex != 1 {
		t.Errorf("EditIndex = %d, want 1", err.EditIndex)
	}
}

func TestApplyAllOrderIsSetOrder(t *testing.T) {
	content := []byte("value\n")
	first := mkPatch("a-first", Edit{Find: "value", Replace: "one"})
	second := mkPatch("b-second", Edit{Find: "one", Replace: "two"})

	got, applied, err := ApplyAll(content, []*Patch{first, second})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "two\n" {
		t.Errorf("got %q, want %q", got, "two\n")
	}
	if len(applied) != 2 || applied[0] != "a-first" || applied[1] != "b-second" {
		t.Errorf("applied = %v, want [a-first b-second]", applied)
	}
}

func TestApplyAllNoPatchesIsPassthrough(t *testing.T) {
	content := []byte("untouched\n")
	got, applied, err := ApplyAll(content, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "untouched\n" {
		t.Errorf("got %q, want passthrough", got)
	}
	if applied != nil {
		t.Errorf("applied = %v, want nil", applied)
	}
}

func TestApplyHintTrailingNewline(t *testing.T) {
	// YAML `find: |` appends a newline the user did not intend.
	content := []byte("prefix Always install with npm. suffix\n")
	p := mkPatch("rad", Edit{Find: "Always install with npm.\n", Replace: "x"})

	_, err := Apply(content, p)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Hint, "|-") {
		t.Errorf("Hint = %q, want it to suggest `find: |-`", err.Hint)
	}
}

func TestApplyHintLineEndings(t *testing.T) {
	content := []byte("first\r\nsecond\r\n")
	p := mkPatch("crlf", Edit{Find: "first\nsecond", Replace: "x"})

	_, err := Apply(content, p)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Hint, "CRLF") {
		t.Errorf("Hint = %q, want it to mention line endings", err.Hint)
	}
}

func TestApplyOverlappingFindCountsNonOverlapping(t *testing.T) {
	// "aa" occurs twice by overlap in "aaa" but once non-overlapping, so this
	// applies rather than erroring. Documented quirk.
	content := []byte("aaa")
	p := mkPatch("ov", Edit{Find: "aa", Replace: "b"})

	got, err := Apply(content, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "ba" {
		t.Errorf("got %q, want %q", got, "ba")
	}
}

func writePatchFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating dir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

const minimalPatch = `version: 1
file: f.md
edits:
  - find: a
    replace: b
`

func TestLoadDirMissingDirIsEmptySet(t *testing.T) {
	set, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist"), "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if set.Len() != 0 {
		t.Errorf("Len() = %d, want 0", set.Len())
	}
	if set.SourceName() != "up" {
		t.Errorf("SourceName() = %q, want %q", set.SourceName(), "up")
	}
}

func TestLoadDirParsesAndDerivesFields(t *testing.T) {
	dir := t.TempDir()
	writePatchFile(t, dir, "rad-skill.yaml", `version: 1
file: skills/rad/SKILL.md
description: Use pnpm
edits:
  - find: npm
    replace: pnpm
`)

	set, err := LoadDir(dir, "upstream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, ok := set.Get("rad-skill")
	if !ok {
		t.Fatal("patch 'rad-skill' not found")
	}
	if p.SourceName != "upstream" {
		t.Errorf("SourceName = %q, want %q", p.SourceName, "upstream")
	}
	if p.File != "skills/rad/SKILL.md" {
		t.Errorf("File = %q, want %q", p.File, "skills/rad/SKILL.md")
	}
	if p.BlessedPath != filepath.Join(dir, "rad-skill.blessed") {
		t.Errorf("BlessedPath = %q", p.BlessedPath)
	}
	if p.FileRef() != "upstream:skills/rad/SKILL.md" {
		t.Errorf("FileRef() = %q", p.FileRef())
	}
}

func TestLoadDirWalksSubdirectories(t *testing.T) {
	// A patch one level down must not be silently ignored - a quiet no-op is
	// the exact failure this feature exists to prevent.
	dir := t.TempDir()
	writePatchFile(t, dir, "top.yaml", minimalPatch)
	writePatchFile(t, dir, "nested/deep.yaml", minimalPatch)

	set, err := LoadDir(dir, "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if set.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", set.Len())
	}
	p, ok := set.Get("nested/deep")
	if !ok {
		t.Fatalf("nested patch not found; names = %v", names(set.All()))
	}
	if p.BlessedPath != filepath.Join(dir, "nested", "deep.blessed") {
		t.Errorf("BlessedPath = %q, want it beside the patch", p.BlessedPath)
	}
}

func TestLoadDirOrderIsPathSort(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"c.yaml", "a.yaml", "b.yaml"} {
		writePatchFile(t, dir, name, minimalPatch)
	}

	set, err := LoadDir(dir, "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"a", "b", "c"}
	for i, p := range set.All() {
		if p.Name != want[i] {
			t.Fatalf("order = %v, want %v", names(set.All()), want)
		}
	}
}

func TestLoadDirIgnoresNonYAML(t *testing.T) {
	dir := t.TempDir()
	writePatchFile(t, dir, "p.yaml", minimalPatch)
	writePatchFile(t, dir, "p.blessed", "some snapshot content")
	writePatchFile(t, dir, "notes.txt", "hello")

	set, err := LoadDir(dir, "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if set.Len() != 1 {
		t.Errorf("Len() = %d, want 1", set.Len())
	}
}

func TestLoadDirDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	writePatchFile(t, dir, "dup.yaml", minimalPatch)
	writePatchFile(t, dir, "dup.yml", minimalPatch)

	_, err := LoadDir(dir, "up")
	if err == nil {
		t.Fatal("expected a duplicate-name error")
	}
	if !strings.Contains(err.Error(), "duplicate patch name") {
		t.Errorf("error = %q, want it to mention duplicate patch name", err)
	}
}

func TestParseFileValidation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"valid", "version: 1\nfile: f.md\nedits:\n  - find: a\n    replace: b\n", ""},
		{"missing version", "file: f.md\nedits:\n  - find: a\n    replace: b\n", "unsupported version"},
		{"future version", "version: 2\nfile: f.md\nedits:\n  - find: a\n    replace: b\n", "unsupported version"},
		{"missing file", "version: 1\nedits:\n  - find: a\n    replace: b\n", "'file' is required"},
		{"blank file", "version: 1\nfile: \"   \"\nedits:\n  - find: a\n    replace: b\n", "'file' is required"},
		{"absolute path", "version: 1\nfile: \"/etc/passwd\"\nedits:\n  - find: a\n    replace: b\n", "cannot be absolute"},
		{"path traversal", "version: 1\nfile: \"../../etc/passwd\"\nedits:\n  - find: a\n    replace: b\n", "path traversal"},
		{"no edits", "version: 1\nfile: f.md\n", "at least one edit"},
		{"empty find", "version: 1\nfile: f.md\nedits:\n  - find: \"\"\n    replace: b\n", "'find' cannot be empty"},
		{"malformed yaml", "version: 1\nfile: [unclosed\n", "parsing patch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writePatchFile(t, dir, "p.yaml", tt.content)

			_, err := ParseFile(filepath.Join(dir, "p.yaml"), dir, "up")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadBlessed(t *testing.T) {
	dir := t.TempDir()
	writePatchFile(t, dir, "p.yaml", minimalPatch)
	p, err := ParseFile(filepath.Join(dir, "p.yaml"), dir, "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok, err := p.LoadBlessed(); err != nil || ok {
		t.Fatalf("LoadBlessed() = (_, %v, %v), want (_, false, nil) when unblessed", ok, err)
	}

	writePatchFile(t, dir, "p.blessed", "pristine upstream\n")
	data, ok, err := p.LoadBlessed()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true once the snapshot exists")
	}
	if string(data) != "pristine upstream\n" {
		t.Errorf("data = %q", data)
	}
}

func TestSetForFileAndFiles(t *testing.T) {
	dir := t.TempDir()
	writePatchFile(t, dir, "a.yaml", "version: 1\nfile: skills/rad/SKILL.md\nedits:\n  - find: a\n    replace: b\n")
	writePatchFile(t, dir, "b.yaml", "version: 1\nfile: skills/rad/SKILL.md\nedits:\n  - find: b\n    replace: c\n")
	writePatchFile(t, dir, "c.yaml", "version: 1\nfile: personas/other.md\nedits:\n  - find: a\n    replace: b\n")

	set, err := LoadDir(dir, "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	shared := set.ForFile("skills/rad/SKILL.md")
	if len(shared) != 2 || shared[0].Name != "a" || shared[1].Name != "b" {
		t.Errorf("ForFile = %v, want [a b] in application order", names(shared))
	}
	if got := set.ForFile("nothing/here.md"); got != nil {
		t.Errorf("ForFile(unpatched) = %v, want nil", names(got))
	}

	// Files drives materialization, so it must be deduplicated and ordered.
	files := set.Files()
	if len(files) != 2 || files[0] != "skills/rad/SKILL.md" || files[1] != "personas/other.md" {
		t.Errorf("Files() = %v, want [skills/rad/SKILL.md personas/other.md]", files)
	}
}

func TestNilSetIsUsable(t *testing.T) {
	var s *Set
	if s.Len() != 0 || s.All() != nil || s.ForFile("b") != nil || s.Files() != nil || s.SourceName() != "" {
		t.Error("nil Set should behave as empty")
	}
	if _, ok := s.Get("x"); ok {
		t.Error("nil Set Get should report not-found")
	}
}

func names(ps []*Patch) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}
