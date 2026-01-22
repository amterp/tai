// Package template provides simple variable substitution for asset files.
package template

import (
	"os"
	"regexp"
	"strings"
)

// Variables holds the template variables available for substitution.
type Variables map[string]string

// Engine handles template processing.
type Engine struct {
	variables Variables
}

// NewEngine creates a new template engine with the given variables.
func NewEngine(vars Variables) *Engine {
	return &Engine{variables: vars}
}

// varPattern matches {{variable}} patterns.
var varPattern = regexp.MustCompile(`\{\{(\w+)\}\}`)

// Process applies template substitution to content.
// Variables are referenced as {{variable_name}}.
// The special variable {{existing}} is replaced with the content of targetPath.
func (e *Engine) Process(content []byte, targetPath string) ([]byte, error) {
	result := string(content)

	// Find all variable references
	matches := varPattern.FindAllStringSubmatch(result, -1)
	for _, match := range matches {
		placeholder := match[0] // e.g., "{{existing}}"
		varName := match[1]     // e.g., "existing"

		var replacement string
		if varName == "existing" {
			// Special case: read existing file content
			existing, err := readExistingContent(targetPath)
			if err != nil {
				// If file doesn't exist, replace with empty string
				replacement = ""
			} else {
				replacement = existing
			}
		} else if val, ok := e.variables[varName]; ok {
			replacement = val
		} else {
			// Unknown variable - leave as-is (fail-safe)
			continue
		}

		result = strings.ReplaceAll(result, placeholder, replacement)
	}

	return []byte(result), nil
}

// readExistingContent reads the content of an existing file.
func readExistingContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// HasTemplateVars checks if content contains any template variables.
func HasTemplateVars(content []byte) bool {
	return varPattern.Match(content)
}

// HasExistingVar checks if content contains the {{existing}} variable.
func HasExistingVar(content []byte) bool {
	return strings.Contains(string(content), "{{existing}}")
}

// ExtractVariables returns all variable names found in content.
func ExtractVariables(content []byte) []string {
	matches := varPattern.FindAllStringSubmatch(string(content), -1)
	seen := make(map[string]bool)
	var vars []string

	for _, match := range matches {
		varName := match[1]
		if !seen[varName] {
			seen[varName] = true
			vars = append(vars, varName)
		}
	}

	return vars
}
