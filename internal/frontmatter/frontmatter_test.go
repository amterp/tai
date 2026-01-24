package frontmatter

import (
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

