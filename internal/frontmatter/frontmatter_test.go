package frontmatter

import (
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

func TestRender(t *testing.T) {
	fields := map[string]string{
		"name":        "Senior SWE",
		"description": "A senior engineer role",
	}
	body := []byte("# Content\n")
	order := []string{"name", "description"}

	result := Render(fields, order, body)
	expected := "---\nname: Senior SWE\ndescription: A senior engineer role\n---\n\n# Content\n"

	if string(result) != expected {
		t.Errorf("Render = %q, want %q", string(result), expected)
	}
}

func TestRenderEmptyBody(t *testing.T) {
	fields := map[string]string{"name": "Test"}
	result := Render(fields, []string{"name"}, nil)
	expected := "---\nname: Test\n---\n"

	if string(result) != expected {
		t.Errorf("Render = %q, want %q", string(result), expected)
	}
}

func TestRenderUnorderedFields(t *testing.T) {
	fields := map[string]string{
		"zebra": "last",
		"alpha": "first",
	}
	result := Render(fields, nil, []byte("body"))
	expected := "---\nalpha: first\nzebra: last\n---\n\nbody"

	if string(result) != expected {
		t.Errorf("Render = %q, want %q", string(result), expected)
	}
}

func TestRoundTrip(t *testing.T) {
	original := []byte("---\nname: Test\ndescription: A test\n---\n\n# Body content\n")

	parsed, err := Parse(original)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	rendered := Render(parsed.Metadata, []string{"name", "description"}, parsed.Body)

	if string(rendered) != string(original) {
		t.Errorf("Round trip failed:\n  got:  %q\n  want: %q", string(rendered), string(original))
	}
}

func TestRenderQuotesColonSpace(t *testing.T) {
	fields := map[string]string{
		"name":        "Test Skill",
		"description": "Examples: user: hello, system: world",
	}
	body := []byte("# Content\n")

	result := Render(fields, []string{"name", "description"}, body)

	// The output should be valid YAML that can be parsed back
	parsed, err := Parse(result)
	if err != nil {
		t.Fatalf("Render produced invalid YAML: %v\nOutput was:\n%s", err, result)
	}

	// Verify the value survived the round trip
	if parsed.Metadata["description"] != fields["description"] {
		t.Errorf("description = %q, want %q", parsed.Metadata["description"], fields["description"])
	}
}

func TestRoundTripWithColonSpace(t *testing.T) {
	fields := map[string]string{
		"name":        "Test",
		"description": "Format: key: value pairs",
	}
	body := []byte("Body content\n")

	// Render then parse
	rendered := Render(fields, []string{"name", "description"}, body)
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

