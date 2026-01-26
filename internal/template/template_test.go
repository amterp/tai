package template

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestEngineProcessUnknownVar(t *testing.T) {
	engine := NewEngine(Variables{})

	content := []byte("Hello {{unknown}}")
	result, err := engine.Process(content, "")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Unknown variables are left as-is
	expected := "Hello {{unknown}}"
	if string(result) != expected {
		t.Errorf("result = %q, want %q", string(result), expected)
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

func TestHasTemplateVars(t *testing.T) {
	tests := []struct {
		content  string
		expected bool
	}{
		{"No variables here", false},
		{"Has {{variable}}", true},
		{"Multiple {{one}} and {{two}}", true},
		{"Partial {{ not valid }}", false},
		{"{{existing}} is special", true},
		{"Empty {{}}", false}, // Empty variable name
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			result := HasTemplateVars([]byte(tt.content))
			if result != tt.expected {
				t.Errorf("HasTemplateVars(%q) = %v, want %v", tt.content, result, tt.expected)
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
		{"{{name}} but not existing", false},
		{"Multiple {{existing}} instances {{existing}}", true},
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

func TestExtractVariables(t *testing.T) {
	tests := []struct {
		content  string
		expected []string
	}{
		{"No variables", nil},
		{"One {{name}}", []string{"name"}},
		{"Two {{name}} and {{version}}", []string{"name", "version"}},
		{"Duplicate {{name}} and {{name}}", []string{"name"}},
		{"Including {{existing}}", []string{"existing"}},
	}

	for _, tt := range tests {
		t.Run(tt.content, func(t *testing.T) {
			result := ExtractVariables([]byte(tt.content))
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ExtractVariables(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
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
		name     string
		vars     Variables
		input    string
		expected string
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
			name:     "unknown variable left as-is",
			vars:     Variables{},
			input:    "{{unknown}}",
			expected: "{{unknown}}",
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
			if err != nil {
				t.Fatalf("ProcessString failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("ProcessString(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
