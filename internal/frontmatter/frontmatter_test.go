package frontmatter

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseWithFrontmatter(t *testing.T) {
	input := []byte("---\nname: Senior SWE\ndescription: Experienced engineer\n---\n\n# Content\n")

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Metadata == nil {
		t.Fatal("Metadata is nil")
	}
	if parsed.Metadata["name"] != "Senior SWE" {
		t.Errorf("name = %q, want %q", parsed.Metadata["name"], "Senior SWE")
	}
	if parsed.Metadata["description"] != "Experienced engineer" {
		t.Errorf("description = %q, want %q", parsed.Metadata["description"], "Experienced engineer")
	}
	if string(parsed.Body) != "# Content\n" {
		t.Errorf("Body = %q, want %q", string(parsed.Body), "# Content\n")
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	input := []byte("# Just a heading\n\nSome content.\n")

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", parsed.Metadata)
	}
	if string(parsed.Body) != string(input) {
		t.Errorf("Body = %q, want %q", string(parsed.Body), string(input))
	}
}

func TestParseNoClosingDelimiter(t *testing.T) {
	input := []byte("---\nname: Orphan\nNo closing delimiter here\n")

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", parsed.Metadata)
	}
	if string(parsed.Body) != string(input) {
		t.Errorf("Body should be full input")
	}
}

func TestParseMalformedYAML(t *testing.T) {
	input := []byte("---\n: invalid: yaml: here\n---\n\nBody\n")

	_, err := Parse(input)
	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}

func TestParseMalformedYAML_MappingValuesError_IncludesHint(t *testing.T) {
	// This pattern triggers "mapping values are not allowed in this context"
	input := []byte("---\nname: test\ndescription: Examples: user: hello\n---\n\nBody\n")

	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "Hint:") {
		t.Errorf("error should contain hint, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, ">-") {
		t.Errorf("error should mention block scalar syntax, got: %v", errMsg)
	}
}

func TestParseEmptyFrontmatter(t *testing.T) {
	input := []byte("---\n---\n\n# Body\n")

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Metadata == nil {
		t.Fatal("Metadata is nil, want empty map")
	}
	if len(parsed.Metadata) != 0 {
		t.Errorf("len(Metadata) = %d, want 0", len(parsed.Metadata))
	}
	if string(parsed.Body) != "# Body\n" {
		t.Errorf("Body = %q, want %q", string(parsed.Body), "# Body\n")
	}
}

func TestParseBodyImmediatelyAfterClosing(t *testing.T) {
	input := []byte("---\nname: Test\n---\nImmediate body\n")

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Metadata["name"] != "Test" {
		t.Errorf("name = %q", parsed.Metadata["name"])
	}
	if string(parsed.Body) != "Immediate body\n" {
		t.Errorf("Body = %q, want %q", string(parsed.Body), "Immediate body\n")
	}
}

// stringFields builds Fields from key/value pairs, in the order given.
func stringFields(pairs ...string) Fields {
	var f Fields
	for i := 0; i < len(pairs); i += 2 {
		f.Set(pairs[i], pairs[i+1])
	}
	return f
}

func TestRender(t *testing.T) {
	fields := stringFields("name", "Senior SWE", "description", "A senior engineer role")
	body := []byte("# Content\n")

	result := Render(fields, body)
	expected := "---\nname: Senior SWE\ndescription: A senior engineer role\n---\n\n# Content\n"

	if string(result) != expected {
		t.Errorf("Render = %q, want %q", string(result), expected)
	}
}

func TestRenderEmptyBody(t *testing.T) {
	result := Render(stringFields("name", "Test"), nil)
	expected := "---\nname: Test\n---\n"

	if string(result) != expected {
		t.Errorf("Render = %q, want %q", string(result), expected)
	}
}

func TestFieldsSetReplacesInPlace(t *testing.T) {
	fields := stringFields("name", "Test", "description", "Original", "tools", "Read")
	fields.Set("description", "Replaced")

	result := Render(fields, nil)
	expected := "---\nname: Test\ndescription: Replaced\ntools: Read\n---\n"

	if string(result) != expected {
		t.Errorf("Render = %q, want %q", string(result), expected)
	}
}

func TestFieldsSetStringSkipsEmpty(t *testing.T) {
	var fields Fields
	fields.Set("name", "Test")
	fields.SetString("description", "")

	if fields.Len() != 1 {
		t.Errorf("Len = %d, want 1", fields.Len())
	}
}

func TestFieldsMergeAppendsSorted(t *testing.T) {
	fields := stringFields("name", "Test")
	fields.Merge(map[string]any{"zebra": "last", "alpha": "first"})

	result := Render(fields, []byte("body"))
	expected := "---\nname: Test\nalpha: first\nzebra: last\n---\n\nbody"

	if string(result) != expected {
		t.Errorf("Render = %q, want %q", string(result), expected)
	}
}

func TestFieldsMergeOverridesInPlace(t *testing.T) {
	fields := stringFields("name", "Test", "description", "tai's description", "tools", "Read")
	fields.Merge(map[string]any{"description": "the target's own description"})

	result := Render(fields, nil)
	expected := "---\nname: Test\ndescription: the target's own description\ntools: Read\n---\n"

	if string(result) != expected {
		t.Errorf("Render = %q, want %q", string(result), expected)
	}
}

func TestRenderNestedValues(t *testing.T) {
	fields := stringFields("name", "Test")
	fields.Merge(map[string]any{
		"disable-model-invocation": true,
		"allowed-tools":            []any{"Read", "Bash"},
		"hooks":                    map[string]any{"stop": "cleanup.sh"},
	})

	rendered := Render(fields, []byte("body\n"))
	parsed, err := Parse(rendered)
	if err != nil {
		t.Fatalf("Render produced invalid YAML: %v\nOutput was:\n%s", err, rendered)
	}

	// Nested values survive as YAML structure, not as Go's %v formatting.
	for _, want := range []string{"allowed-tools:\n  - Read\n  - Bash", "hooks:\n  stop: cleanup.sh", "disable-model-invocation: true"} {
		if !strings.Contains(string(rendered), want) {
			t.Errorf("rendered output missing %q, got:\n%s", want, rendered)
		}
	}

	if string(parsed.Body) != "body\n" {
		t.Errorf("Body = %q, want %q", string(parsed.Body), "body\n")
	}
}

func TestRoundTrip(t *testing.T) {
	original := []byte("---\nname: Test\ndescription: A test\n---\n\n# Body content\n")

	parsed, err := Parse(original)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	rendered := Render(stringFields(
		"name", parsed.Metadata["name"],
		"description", parsed.Metadata["description"],
	), parsed.Body)

	if string(rendered) != string(original) {
		t.Errorf("Round trip failed:\n  got:  %q\n  want: %q", string(rendered), string(original))
	}
}

func TestRenderQuotesColonSpace(t *testing.T) {
	desc := "Examples: user: hello, system: world"
	body := []byte("# Content\n")

	result := Render(stringFields("name", "Test Skill", "description", desc), body)

	// The output should be valid YAML that can be parsed back
	parsed, err := Parse(result)
	if err != nil {
		t.Fatalf("Render produced invalid YAML: %v\nOutput was:\n%s", err, result)
	}

	// Verify the value survived the round trip
	if parsed.Metadata["description"] != desc {
		t.Errorf("description = %q, want %q", parsed.Metadata["description"], desc)
	}
}

func TestRoundTripWithColonSpace(t *testing.T) {
	fields := map[string]string{
		"name":        "Test",
		"description": "Format: key: value pairs",
	}
	body := []byte("Body content\n")

	// Render then parse
	rendered := Render(stringFields(
		"name", fields["name"],
		"description", fields["description"],
	), body)
	parsed, err := Parse(rendered)
	if err != nil {
		t.Fatalf("Parse failed after render: %v\nRendered:\n%s", err, rendered)
	}

	// Verify all fields preserved
	for key, want := range fields {
		if got := parsed.Metadata[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	if string(parsed.Body) != string(body) {
		t.Errorf("Body = %q, want %q", string(parsed.Body), string(body))
	}
}

func TestParseDependsArray(t *testing.T) {
	input := []byte(`---
name: Code Review
description: Reviews code
depends:
  - delegates/bug-hunter.md
  - delegates/code-reviewer.md
  - skills/council-participant
---

# Body
`)

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Metadata["name"] != "Code Review" {
		t.Errorf("name = %q, want %q", parsed.Metadata["name"], "Code Review")
	}

	// Depends should not be in metadata
	if _, ok := parsed.Metadata["depends"]; ok {
		t.Error("depends should not be in Metadata map")
	}

	// Check parsed depends
	if len(parsed.Depends) != 3 {
		t.Fatalf("len(Depends) = %d, want 3", len(parsed.Depends))
	}

	expected := []string{
		"delegates/bug-hunter.md",
		"delegates/code-reviewer.md",
		"skills/council-participant",
	}

	for i, want := range expected {
		got := parsed.Depends[i].Path
		if got != want {
			t.Errorf("Depends[%d].Path = %q, want %q", i, got, want)
		}
	}
}

func TestParseDependsWithSource(t *testing.T) {
	input := []byte(`---
name: Test
depends:
  - other-source:delegates/bug-hunter.md
---
`)

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(parsed.Depends) != 1 {
		t.Fatalf("len(Depends) = %d, want 1", len(parsed.Depends))
	}

	got := parsed.Depends[0].Path
	want := "other-source:delegates/bug-hunter.md"
	if got != want {
		t.Errorf("Depends[0].Path = %q, want %q", got, want)
	}
}

func TestParseDependsEmpty(t *testing.T) {
	input := []byte(`---
name: Test
depends: []
---
`)

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(parsed.Depends) != 0 {
		t.Errorf("len(Depends) = %d, want 0", len(parsed.Depends))
	}
}

func TestParseDependsInvalidNotArray(t *testing.T) {
	input := []byte(`---
name: Test
depends: "not-an-array"
---
`)

	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for non-array depends")
	}
	if !strings.Contains(err.Error(), "must be an array") {
		t.Errorf("error = %q, should contain 'must be an array'", err.Error())
	}
}

func TestParseDependsInvalidNotString(t *testing.T) {
	input := []byte(`---
name: Test
depends:
  - delegate: foo
---
`)

	_, err := Parse(input)
	if err == nil {
		t.Fatal("expected error for non-string depends entry")
	}
	if !strings.Contains(err.Error(), "must be a string path") {
		t.Errorf("error = %q, should contain 'must be a string path'", err.Error())
	}
}

func TestParseNoDepends(t *testing.T) {
	input := []byte(`---
name: Test
description: No depends
---
`)

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Depends != nil {
		t.Errorf("Depends = %v, want nil", parsed.Depends)
	}
}

func TestParseNative(t *testing.T) {
	input := []byte(`---
name: parallel-work
description: A skill
native:
  claude:
    disable-model-invocation: true
    allowed-tools: [Read, Bash]
  codex:
    argument-hint: "<slot>"
---

# Body
`)

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if got := parsed.Native["claude"]["disable-model-invocation"]; got != true {
		t.Errorf("native.claude.disable-model-invocation = %v, want true", got)
	}
	if got := parsed.Native["codex"]["argument-hint"]; got != "<slot>" {
		t.Errorf("native.codex.argument-hint = %v, want %q", got, "<slot>")
	}

	// native is tai's own key and must not leak into the fields adapters read.
	if _, ok := parsed.Metadata[FieldNative]; ok {
		t.Errorf("Metadata still holds %q: %v", FieldNative, parsed.Metadata)
	}
}

func TestParseNativeAbsent(t *testing.T) {
	parsed, err := Parse([]byte("---\nname: Test\n---\n\nbody\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if parsed.Native != nil {
		t.Errorf("Native = %v, want nil", parsed.Native)
	}
}

func TestParseNativeInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not a map", "---\nname: Test\nnative: claude\n---\n\nbody\n"},
		{"list", "---\nname: Test\nnative:\n  - claude\n---\n\nbody\n"},
		{"target maps to a scalar", "---\nname: Test\nnative:\n  claude: true\n---\n\nbody\n"},
		{"target maps to a list", "---\nname: Test\nnative:\n  claude: [a, b]\n---\n\nbody\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.input))
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if !strings.Contains(err.Error(), "native") {
				t.Errorf("error = %q, want it to mention native", err)
			}
		})
	}
}

func TestIsTaiField(t *testing.T) {
	for _, field := range []string{FieldName, FieldDescription, FieldTools, FieldModel, FieldDepends, FieldNative} {
		if !IsTaiField(field) {
			t.Errorf("IsTaiField(%q) = false, want true", field)
		}
	}
	for _, field := range []string{"disable-model-invocation", "allowed-tools", "license"} {
		if IsTaiField(field) {
			t.Errorf("IsTaiField(%q) = true, want false", field)
		}
	}
}

// Skills written for a target often carry a nested `metadata:` block. Parsing
// must keep its structure rather than flattening it to "map[author:...]".
func TestParseNestedUnknownField(t *testing.T) {
	input := []byte(`---
name: demo
description: A demo skill
metadata:
  author: Someone
  tags:
    - a
    - b
version: 2
---

Body
`)

	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	wantMetadata := map[string]string{"name": "demo", "description": "A demo skill", "version": "2"}
	if !reflect.DeepEqual(parsed.Metadata, wantMetadata) {
		t.Errorf("Metadata = %#v, want %#v", parsed.Metadata, wantMetadata)
	}
	wantStructured := map[string]any{
		"metadata": map[string]any{"author": "Someone", "tags": []any{"a", "b"}},
	}
	if !reflect.DeepEqual(parsed.Structured, wantStructured) {
		t.Errorf("Structured = %#v, want %#v", parsed.Structured, wantStructured)
	}
	if string(parsed.Body) != "Body\n" {
		t.Errorf("Body = %q", parsed.Body)
	}
}

// A list or map is kept as parsed wherever it appears, including under a tai
// field (the installer drops that with a warning) and under a map with
// non-string keys, which yaml decodes as map[any]any.
func TestParseKeepsStructuredValues(t *testing.T) {
	parsed, err := Parse([]byte("---\nname: demo\ntools: [Read, Grep]\nnumbered:\n  1: x\n---\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if _, ok := parsed.Metadata["tools"]; ok {
		t.Errorf("tools was flattened into Metadata: %q", parsed.Metadata["tools"])
	}
	if _, ok := parsed.Metadata["numbered"]; ok {
		t.Errorf("numbered was flattened into Metadata: %q", parsed.Metadata["numbered"])
	}
	if !reflect.DeepEqual(parsed.Structured["tools"], []any{"Read", "Grep"}) {
		t.Errorf("Structured[tools] = %#v", parsed.Structured["tools"])
	}
	if _, ok := parsed.Structured["numbered"]; !ok {
		t.Errorf("Structured = %#v, want numbered kept", parsed.Structured)
	}
}
