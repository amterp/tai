package template

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEngineProcessSimple(t *testing.T) {
	engine := NewEngine(Variables{
		"name":    "test-project",
		"version": "1.0.0",
	})

	content := []byte("Project: {{name}}, Version: {{version}}")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "Project: test-project, Version: 1.0.0"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestEngineProcessSimpleWithSpaces(t *testing.T) {
	engine := NewEngine(Variables{
		"name": "test-project",
	})

	content := []byte("Project: {{ name }}")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "Project: test-project"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestEngineProcessUnknownVar(t *testing.T) {
	engine := NewEngine(Variables{})

	content := []byte("Hello {{ unknown }}")
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for undefined variable, got nil")
	}
	if !strings.Contains(err.Error(), "undefined template variable") {
		t.Errorf("error = %q, should mention undefined template variable", err.Error())
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error = %q, should mention the variable name 'unknown'", err.Error())
	}
}

func TestEngineProcessExisting(t *testing.T) {
	// Create a temp file with existing content
	tmpDir := t.TempDir()
	existingPath := filepath.Join(tmpDir, "existing.md")
	existingContent := "# Existing Content\nThis was already here."
	if err := os.WriteFile(existingPath, []byte(existingContent), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(Variables{})

	content := []byte("# New Header\n\n{{existing}}\n\n# New Footer")
	result, err := engine.Process(content, existingPath)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "# New Header\n\n# Existing Content\nThis was already here.\n\n# New Footer"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestEngineProcessExistingWithSpaces(t *testing.T) {
	tmpDir := t.TempDir()
	existingPath := filepath.Join(tmpDir, "existing.md")
	if err := os.WriteFile(existingPath, []byte("CONTENT"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(Variables{})

	content := []byte("Before {{ existing }} After")
	result, err := engine.Process(content, existingPath)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "Before CONTENT After"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestEngineProcessExistingMissing(t *testing.T) {
	engine := NewEngine(Variables{})

	content := []byte("Before {{existing}} After")
	result, err := engine.Process(content, "/nonexistent/file.md")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Missing file results in empty replacement
	expected := "Before  After"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestEngineProcessMultipleSameVar(t *testing.T) {
	engine := NewEngine(Variables{
		"name": "tai",
	})

	content := []byte("{{name}} is called {{name}}")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "tai is called tai"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestEngineProcessMixedVars(t *testing.T) {
	// Create temp file for {{existing}}
	tmpDir := t.TempDir()
	existingPath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(existingPath, []byte("EXISTING"), 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(Variables{
		"name": "project",
	})

	content := []byte("Name: {{name}}, Previous: {{existing}}")
	result, err := engine.Process(content, existingPath)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "Name: project, Previous: EXISTING"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestHasTemplateSyntax(t *testing.T) {
	tests := []struct {
		content  string
		expected bool
	}{
		{"No variables here", false},
		{"Has {{variable}}", true},
		{"Has {{ variable }}", true},
		{"Multiple {{one}} and {{two}}", true},
		{"{{existing}} is special", true},
		{"Tag {% if x %}yes{% endif %}", true},
		{"Comment {# note #}", true},
		{"Single braces {not_template}", false},
		{"Empty content", false},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			result := HasTemplateSyntax([]byte(tt.content))
			if result != tt.expected {
				t.Errorf("HasTemplateSyntax(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestHasExistingVar(t *testing.T) {
	tests := []struct {
		content  string
		expected bool
	}{
		{"No existing", false},
		{"Has {{existing}}", true},
		{"Has {{ existing }}", true},
		{"Has {{  existing  }}", true},
		{"{{name}} but not existing", false},
		{"Multiple {{existing}} instances {{existing}}", true},
		// Verbatim blocks are ignored
		{"{% verbatim %}{{ existing }}{% endverbatim %}", false},
		{"{% verbatim %}{{existing}}{% endverbatim %}", false},
		{"before {% verbatim %}{{ existing }}{% endverbatim %} after", false},
		// Real existing outside verbatim still detected
		{"{{ existing }} {% verbatim %}{{ existing }}{% endverbatim %}", true},
		{"{% verbatim %}docs{% endverbatim %} {{ existing }}", true},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			result := HasExistingVar([]byte(tt.content))
			if result != tt.expected {
				t.Errorf("HasExistingVar(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestHasTemplateSyntaxDetectsVerbatim(t *testing.T) {
	// Verbatim tags themselves ARE template syntax that the engine needs to process
	if !HasTemplateSyntax([]byte("{% verbatim %}{{ name }}{% endverbatim %}")) {
		t.Error("HasTemplateSyntax should detect verbatim tags as template syntax")
	}

	// Plain text with no syntax
	if HasTemplateSyntax([]byte("plain text no syntax")) {
		t.Error("HasTemplateSyntax should return false for plain text")
	}
}

func TestEngineProcessNoVars(t *testing.T) {
	engine := NewEngine(Variables{})

	content := []byte("Plain content without any variables")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if string(result) != string(content) {
		t.Errorf("result = %q, want %q", string(result), string(content))
	}
}

func TestMergeVariables(t *testing.T) {
	tests := []struct {
		name     string
		sources  []Variables
		expected Variables
	}{
		{
			name:     "empty sources",
			sources:  []Variables{},
			expected: Variables{},
		},
		{
			name:     "single source",
			sources:  []Variables{{"a": "1", "b": "2"}},
			expected: Variables{"a": "1", "b": "2"},
		},
		{
			name: "later wins on collision",
			sources: []Variables{
				{"a": "first", "b": "first"},
				{"a": "second"},
			},
			expected: Variables{"a": "second", "b": "first"},
		},
		{
			name: "three sources",
			sources: []Variables{
				{"a": "1"},
				{"b": "2"},
				{"c": "3"},
			},
			expected: Variables{"a": "1", "b": "2", "c": "3"},
		},
		{
			name: "nil source skipped",
			sources: []Variables{
				{"a": "1"},
				nil,
				{"b": "2"},
			},
			expected: Variables{"a": "1", "b": "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeVariables(tt.sources...)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("MergeVariables() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestProcessString(t *testing.T) {
	tests := []struct {
		name      string
		vars      Variables
		input     string
		expected  string
		expectErr string // if non-empty, expect error containing this substring
	}{
		{
			name:     "simple substitution",
			vars:     Variables{"name": "claude"},
			input:    "target_{{name}}",
			expected: "target_claude",
		},
		{
			name:     "no variables",
			vars:     Variables{},
			input:    "plain string",
			expected: "plain string",
		},
		{
			name:      "unknown variable errors",
			vars:      Variables{},
			input:     "{{unknown}}",
			expectErr: "undefined template variable",
		},
		{
			name:     "multiple variables",
			vars:     Variables{"a": "1", "b": "2"},
			input:    "{{a}}/{{b}}",
			expected: "1/2",
		},
		{
			name:     "empty string input",
			vars:     Variables{"x": "y"},
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(tt.vars)
			result, err := engine.ProcessString(tt.input)
			if tt.expectErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.expectErr)
				}
				if !strings.Contains(err.Error(), tt.expectErr) {
					t.Fatalf("error = %q, should contain %q", err.Error(), tt.expectErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ProcessString failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("ProcessString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// --- Conditional tests ---

func TestConditionalSimple(t *testing.T) {
	engine := NewEngine(Variables{"target_name": "claude"})

	content := []byte(`{% if target_name == "claude" %}Claude mode{% else %}Other mode{% endif %}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if string(result) != "Claude mode" {
		t.Errorf("result = %q, want %q", string(result), "Claude mode")
	}
}

func TestConditionalElif(t *testing.T) {
	engine := NewEngine(Variables{"target_name": "codex"})

	content := []byte(`{% if target_name == "claude" %}C{% elif target_name == "codex" %}X{% else %}?{% endif %}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if string(result) != "X" {
		t.Errorf("result = %q, want %q", string(result), "X")
	}
}

func TestConditionalElse(t *testing.T) {
	engine := NewEngine(Variables{"target_name": "opencode"})

	content := []byte(`{% if target_name == "claude" %}C{% elif target_name == "codex" %}X{% else %}?{% endif %}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if string(result) != "?" {
		t.Errorf("result = %q, want %q", string(result), "?")
	}
}

func TestConditionalNested(t *testing.T) {
	engine := NewEngine(Variables{"target_name": "claude", "mode": "advanced"})

	content := []byte(`{% if target_name == "claude" %}{% if mode == "advanced" %}full{% else %}basic{% endif %}{% endif %}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if string(result) != "full" {
		t.Errorf("result = %q, want %q", string(result), "full")
	}
}

func TestConditionalMultiline(t *testing.T) {
	engine := NewEngine(Variables{"target_name": "codex"})

	content := []byte("# Header\n{% if target_name == \"codex\" %}\n## Codex Section\n{% endif %}\n# Footer")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "# Header\n\n## Codex Section\n\n# Footer"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestConditionalFalseRemovesContent(t *testing.T) {
	engine := NewEngine(Variables{"target_name": "claude"})

	content := []byte("before{% if target_name == \"codex\" %} included{% endif %} after")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "before after"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

// --- Verbatim (raw) block tests ---

func TestVerbatimBlock(t *testing.T) {
	engine := NewEngine(Variables{"name": "test"})

	content := []byte("{% verbatim %}{{ name }} {% if x %}{% endif %}{% endverbatim %}")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "{{ name }} {% if x %}{% endif %}"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

// --- Include asset tests ---

// mockResolver implements ContentResolver for testing.
type mockResolver struct {
	assets map[string][]byte
}

func (m *mockResolver) ResolveAssetContent(ref string) ([]byte, error) {
	content, ok := m.assets[ref]
	if !ok {
		return nil, fmt.Errorf("asset not found: %s", ref)
	}
	return content, nil
}

func TestIncludeAssetSimple(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{
			"local:personas/test.md": []byte("# Included Content"),
		},
	}

	engine := NewEngineWithResolver(Variables{}, resolver)

	content := []byte(`before {% include_asset "local:personas/test.md" %} after`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "before # Included Content after"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestIncludeAssetWithTemplateVars(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{
			"local:personas/test.md": []byte("Hello {{ name }}!"),
		},
	}

	engine := NewEngineWithResolver(Variables{"name": "world"}, resolver)

	content := []byte(`{% include_asset "local:personas/test.md" %}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "Hello world!"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestIncludeAssetNested(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{
			"local:personas/outer.md": []byte(`Outer[{% include_asset "local:personas/inner.md" %}]`),
			"local:personas/inner.md": []byte("Inner"),
		},
	}

	engine := NewEngineWithResolver(Variables{}, resolver)

	content := []byte(`{% include_asset "local:personas/outer.md" %}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "Outer[Inner]"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestIncludeAssetCycleDetection(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{
			"local:a.md": []byte(`{% include_asset "local:b.md" %}`),
			"local:b.md": []byte(`{% include_asset "local:a.md" %}`),
		},
	}

	engine := NewEngineWithResolver(Variables{}, resolver)

	content := []byte(`{% include_asset "local:a.md" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("error = %q, should contain 'cycle detected'", err.Error())
	}
}

func TestIncludeAssetMissing(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{},
	}

	engine := NewEngineWithResolver(Variables{}, resolver)

	content := []byte(`{% include_asset "local:nonexistent.md" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for missing asset, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, should contain 'not found'", err.Error())
	}
}

func TestIncludeAssetNoResolver(t *testing.T) {
	engine := NewEngine(Variables{})

	content := []byte(`{% include_asset "local:test.md" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error when no resolver, got nil")
	}
	if !strings.Contains(err.Error(), "ContentResolver") {
		t.Errorf("error = %q, should mention ContentResolver", err.Error())
	}
}

func TestIncludeAssetWithConditional(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{
			"local:personas/senior-swe.md": []byte("You are a senior engineer."),
		},
	}

	engine := NewEngineWithResolver(Variables{"target_name": "codex"}, resolver)

	content := []byte(`# Base
{% if target_name == "codex" %}{% include_asset "local:personas/senior-swe.md" %}{% endif %}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "# Base\nYou are a senior engineer."
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}

	// When target is not codex, included content should be absent
	engine2 := NewEngineWithResolver(Variables{"target_name": "claude"}, resolver)
	result2, err := engine2.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected2 := "# Base\n"
	if string(result2) != expected2 {
		t.Errorf("result = %q, want %q", string(result2), expected2)
	}
}

func TestIncludeAssetDefaultSource(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{
			"personas/test.md": []byte("No source prefix"),
		},
	}

	engine := NewEngineWithResolver(Variables{}, resolver)

	content := []byte(`{% include_asset "personas/test.md" %}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	expected := "No source prefix"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

// --- Built-in include is disabled ---

func TestBuiltInIncludeDisabled(t *testing.T) {
	engine := NewEngine(Variables{})

	content := []byte(`{% include "some_file.html" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for built-in include, got nil")
	}
	if !strings.Contains(err.Error(), "sandbox") {
		t.Errorf("error = %q, should mention sandbox", err.Error())
	}
}

func TestIncludeAssetCycleDetectionMixedRefs(t *testing.T) {
	// Asset A includes B with "local:" prefix, B includes A without prefix.
	// Cycle should be detected even though the ref strings differ, because
	// both normalize to "local:personas/a.md".
	resolver := &mockResolver{
		assets: map[string][]byte{
			"personas/a.md":      []byte(`{% include_asset "local:personas/b.md" %}`),
			"local:personas/b.md": []byte(`{% include_asset "personas/a.md" %}`),
		},
	}

	engine := NewEngineWithResolver(Variables{}, resolver)

	content := []byte(`{% include_asset "personas/a.md" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected cycle error with mixed ref forms")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("error = %q, should contain 'cycle detected'", err.Error())
	}
}

func TestIncludeAssetMaxDepth(t *testing.T) {
	// Create a chain of assets deeper than the limit
	assets := make(map[string][]byte)
	for i := 0; i <= maxIncludeDepth+1; i++ {
		ref := fmt.Sprintf("local:assets/%d.md", i)
		nextRef := fmt.Sprintf("local:assets/%d.md", i+1)
		assets[ref] = []byte(fmt.Sprintf(`{%% include_asset "%s" %%}`, nextRef))
	}
	// Terminal asset (never reached)
	assets[fmt.Sprintf("local:assets/%d.md", maxIncludeDepth+2)] = []byte("end")

	resolver := &mockResolver{assets: assets}
	engine := NewEngineWithResolver(Variables{}, resolver)

	content := []byte(`{% include_asset "local:assets/0.md" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for exceeding max include depth")
	}
	if !strings.Contains(err.Error(), "depth") {
		t.Errorf("error = %q, should mention depth", err.Error())
	}
}

func TestIncludeAssetEmptyRef(t *testing.T) {
	engine := NewEngine(Variables{})

	content := []byte(`{% include_asset "" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for empty ref")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %q, should mention empty", err.Error())
	}
}

func TestIncludeAssetAbsolutePath(t *testing.T) {
	engine := NewEngine(Variables{})

	content := []byte(`{% include_asset "/etc/passwd" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("error = %q, should mention absolute", err.Error())
	}
}

// --- HasTemplateVars backward compatibility ---

func TestHasTemplateVarsBackwardCompat(t *testing.T) {
	if !HasTemplateVars([]byte("{{name}}")) {
		t.Error("HasTemplateVars should detect {{name}}")
	}
	if HasTemplateVars([]byte("no template")) {
		t.Error("HasTemplateVars should not detect plain text")
	}
}

// --- Env filter tests ---

func TestEnvFilterPresent(t *testing.T) {
	t.Setenv("TAI_TEST_PRESENT_VAR", "present-value")
	engine := NewEngine(Variables{})
	content := []byte(`{{ "TAI_TEST_PRESENT_VAR"|env }}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if string(result) != "present-value" {
		t.Errorf("result = %q, want %q", string(result), "present-value")
	}
}

func TestEnvFilterMissing(t *testing.T) {
	engine := NewEngine(Variables{})
	content := []byte(`[{{ "TAI_TEST_NONEXISTENT_VAR_12345"|env }}]`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if string(result) != "[]" {
		t.Errorf("result = %q, want %q", string(result), "[]")
	}
}

func TestEnvFilterWithSet(t *testing.T) {
	t.Setenv("TAI_TEST_ENV_VAR", "hello_tai")
	engine := NewEngine(Variables{})
	content := []byte(`{{ "TAI_TEST_ENV_VAR"|env }}`)
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if string(result) != "hello_tai" {
		t.Errorf("result = %q, want %q", string(result), "hello_tai")
	}
}

// --- Undefined variable detection tests ---

func TestUndefinedVarInContent(t *testing.T) {
	engine := NewEngine(Variables{"defined": "yes"})
	content := []byte("{{ defined }} {{ not_defined }}")
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "not_defined") {
		t.Errorf("error = %q, should mention 'not_defined'", err.Error())
	}
}

func TestUndefinedVarMultiple(t *testing.T) {
	engine := NewEngine(Variables{})
	content := []byte("{{ foo }} {{ bar }}")
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for undefined variables")
	}
	if !strings.Contains(err.Error(), "bar") || !strings.Contains(err.Error(), "foo") {
		t.Errorf("error = %q, should mention both 'foo' and 'bar'", err.Error())
	}
}

func TestUndefinedVarInConditional(t *testing.T) {
	engine := NewEngine(Variables{})
	content := []byte("{% if some_flag %}yes{% endif %}")
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for undefined variable in conditional")
	}
	if !strings.Contains(err.Error(), "some_flag") {
		t.Errorf("error = %q, should mention 'some_flag'", err.Error())
	}
}

func TestDefinedEmptyVarInConditional(t *testing.T) {
	engine := NewEngine(Variables{"optional_section": ""})
	content := []byte("before{% if optional_section %} included{% endif %} after")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	expected := "before after"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestDefinedNonEmptyVarInConditional(t *testing.T) {
	engine := NewEngine(Variables{"optional_section": "content"})
	content := []byte("before{% if optional_section %} included{% endif %} after")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	expected := "before included after"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
	}
}

func TestExistingVarNotFlagged(t *testing.T) {
	engine := NewEngine(Variables{})
	content := []byte("{{ existing }}")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if string(result) != "" {
		t.Errorf("result = %q, want empty", string(result))
	}
}

func TestDefinedUserVarResolvesCorrectly(t *testing.T) {
	engine := NewEngine(Variables{"name": "test"})
	content := []byte("{{ name }}")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if string(result) != "test" {
		t.Errorf("result = %q, want %q", string(result), "test")
	}
}

func TestUndefinedVarInIncludeAsset(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{
			"local:personas/test.md": []byte("Hello {{ missing_in_include }}!"),
		},
	}
	engine := NewEngineWithResolver(Variables{}, resolver)
	content := []byte(`{% include_asset "local:personas/test.md" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for undefined variable in included asset")
	}
	if !strings.Contains(err.Error(), "missing_in_include") {
		t.Errorf("error = %q, should mention 'missing_in_include'", err.Error())
	}
}

func TestUndefinedVarInNestedIncludeAsset(t *testing.T) {
	resolver := &mockResolver{
		assets: map[string][]byte{
			"local:outer.md": []byte(`{% include_asset "local:inner.md" %}`),
			"local:inner.md": []byte("{{ deeply_missing }}"),
		},
	}
	engine := NewEngineWithResolver(Variables{}, resolver)
	content := []byte(`{% include_asset "local:outer.md" %}`)
	_, err := engine.Process(content, "")
	if err == nil {
		t.Fatal("expected error for undefined variable in nested included asset")
	}
	if !strings.Contains(err.Error(), "deeply_missing") {
		t.Errorf("error = %q, should mention 'deeply_missing'", err.Error())
	}
}

// --- Reserved variable name validation ---

func TestValidateVarNamesReserved(t *testing.T) {
	err := ValidateVarNames(Variables{"existing": "override"})
	if err == nil {
		t.Fatal("expected error for reserved variable name 'existing'")
	}
	if !strings.Contains(err.Error(), "existing") {
		t.Errorf("error = %q, should mention 'existing'", err.Error())
	}
}

func TestValidateVarNamesTaiPrefix(t *testing.T) {
	err := ValidateVarNames(Variables{"_tai_custom": "value"})
	if err == nil {
		t.Fatal("expected error for _tai_ prefixed variable name")
	}
	if !strings.Contains(err.Error(), "_tai_custom") {
		t.Errorf("error = %q, should mention '_tai_custom'", err.Error())
	}
}

func TestValidateVarNamesOk(t *testing.T) {
	err := ValidateVarNames(Variables{"project_name": "foo", "env": "prod"})
	if err != nil {
		t.Fatalf("expected no error for normal variable names, got: %v", err)
	}
}

// --- knownContextKeys sync test ---

func TestKnownUndefinedIncludesAllContextKeys(t *testing.T) {
	// All internal context keys from tags.go must be in the filter
	for _, key := range []string{
		contextKeyResolver,
		contextKeyIncludeStack,
		contextKeyTemplateSet,
		contextKeyVariables,
	} {
		if !knownUndefined[key] {
			t.Errorf("knownUndefined missing internal context key %q", key)
		}
	}

	// Reserved names must be in the filter
	if !knownUndefined["existing"] {
		t.Error("knownUndefined missing reserved name 'existing'")
	}

	// pongo2 meta-context
	if !knownUndefined[pongo2MetaContextKey] {
		t.Error("knownUndefined missing pongo2 meta-context key")
	}
}

// --- MergeVariables with 4 sources ---

func TestMergeVariablesFourSources(t *testing.T) {
	adapter := Variables{"target_name": "claude", "shared": "adapter"}
	taiYaml := Variables{"project": "my-project", "shared": "tai-yaml"}
	profile := Variables{"env": "prod", "shared": "profile"}
	cli := Variables{"override": "cli-val", "shared": "cli"}

	result := MergeVariables(adapter, taiYaml, profile, cli)

	// Each source's unique var is preserved
	if result["target_name"] != "claude" {
		t.Errorf("target_name = %q, want %q", result["target_name"], "claude")
	}
	if result["project"] != "my-project" {
		t.Errorf("project = %q, want %q", result["project"], "my-project")
	}
	if result["env"] != "prod" {
		t.Errorf("env = %q, want %q", result["env"], "prod")
	}
	if result["override"] != "cli-val" {
		t.Errorf("override = %q, want %q", result["override"], "cli-val")
	}
	// CLI wins on collision
	if result["shared"] != "cli" {
		t.Errorf("shared = %q, want %q (CLI should win)", result["shared"], "cli")
	}
}
