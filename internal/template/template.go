// Package template provides Jinja2-style template processing for asset files,
// powered by pongo2. Supports variable substitution ({{ var }}), conditionals
// ({% if %}/{% elif %}/{% else %}), and asset inlining ({% include_asset %}).
package template

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	pongo2 "github.com/amterp/pongo2/v6"
)

// Variables holds the template variables available for substitution.
type Variables map[string]string

// ContentResolver resolves asset references to their body content (frontmatter stripped).
// The template package defines this interface; the install package implements it.
type ContentResolver interface {
	// ResolveAssetContent resolves an asset reference (e.g. "local:personas/senior-swe.md")
	// and returns its body content with frontmatter stripped.
	ResolveAssetContent(ref string) ([]byte, error)
}

// MergeVariables combines multiple variable maps, with later maps taking precedence.
// Priority order: adapter vars < tai.yaml vars < profile vars < CLI --var.
func MergeVariables(sources ...Variables) Variables {
	result := make(Variables)
	for _, src := range sources {
		for k, v := range src {
			result[k] = v
		}
	}
	return result
}

// Engine handles template processing using pongo2 internally.
type Engine struct {
	variables Variables
	resolver  ContentResolver
	set       *pongo2.TemplateSet
}

// noopLoader prevents pongo2's built-in {% include %} from accessing the filesystem.
// All asset inclusion must go through our controlled {% include_asset %} tag.
type noopLoader struct{}

func (n *noopLoader) Abs(_, name string) string { return name }
func (n *noopLoader) Get(_ string) (io.Reader, error) {
	return nil, fmt.Errorf("built-in include is disabled; use include_asset instead")
}

// NewEngine creates a new template engine with the given variables.
// The engine has no ContentResolver, so {% include_asset %} will error.
func NewEngine(vars Variables) *Engine {
	return &Engine{
		variables: vars,
		set:       taiTemplateSet,
	}
}

// NewEngineWithResolver creates a new template engine with variables and a ContentResolver.
// This enables {% include_asset "ref" %} to resolve and inline other assets.
func NewEngineWithResolver(vars Variables, resolver ContentResolver) *Engine {
	return &Engine{
		variables: vars,
		resolver:  resolver,
		set:       taiTemplateSet,
	}
}

// taiTemplateSet is the shared pongo2 TemplateSet for all tai template engines.
// The set configuration (banned tags, custom tags/filters) is static, so we
// create it once at package init. This moves registration panics to startup
// and avoids per-engine overhead.
var taiTemplateSet = newTaiTemplateSet()

// newTaiTemplateSet creates a pongo2 TemplateSet configured for tai:
// built-in include disabled, tai custom tags and filters registered.
func newTaiTemplateSet() *pongo2.TemplateSet {
	set := pongo2.NewSet("tai", &noopLoader{})
	_ = set.BanTag("include")
	registerTaiTags(set)
	registerTaiFilters(set)
	return set
}

// ProcessString applies template substitution to a string.
// This is a convenience wrapper for processing short strings like dest paths.
// No ContentResolver is used - {% include_asset %} will error if encountered.
func (e *Engine) ProcessString(s string) (string, error) {
	result, err := e.Process([]byte(s), "")
	return string(result), err
}

// Process applies template processing to content.
// Variables are referenced as {{ variable_name }} or {{variable_name}}.
// The special variable {{ existing }} is replaced with the content of targetPath.
// Supports {% if %}, {% elif %}, {% else %}, {% endif %} conditionals,
// and {% include_asset "source:path" %} for inlining other assets.
func (e *Engine) Process(content []byte, targetPath string) ([]byte, error) {
	// Build pongo2 context from variables
	ctx := pongo2.Context{}
	for k, v := range e.variables {
		ctx[k] = v
	}

	// Inject existing file content if applicable
	if targetPath != "" {
		existing, err := readExistingContent(targetPath)
		if err != nil {
			// If file doesn't exist, existing is empty string
			ctx["existing"] = ""
		} else {
			ctx["existing"] = existing
		}
	} else {
		ctx["existing"] = ""
	}

	// Inject state needed by the include_asset tag
	injectTagContext(ctx, e.resolver, e.set, e.variables)

	tpl, err := e.set.FromBytes(content)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	report, err := tpl.ExecuteWriterWithReport(ctx, &buf)
	if err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}

	if report != nil && len(report.UndefinedVars) > 0 {
		unexpected := filterKnownUndefined(report.UndefinedVars)
		if len(unexpected) > 0 {
			return nil, fmt.Errorf("undefined template variable(s): %v", unexpected)
		}
	}

	return buf.Bytes(), nil
}

// pongo2MetaContextKey is the key pongo2 uses for its own internal meta-context.
// It lives in the private context so it shouldn't appear in UndefinedVars,
// but we filter it defensively in case that behavior changes.
const pongo2MetaContextKey = "pongo2"

// knownUndefined is the set of variable names that should never be flagged
// as undefined. Computed once at package init from the context key constants
// in tags.go, reserved names, and pongo2 internals.
var knownUndefined = func() map[string]bool {
	return map[string]bool{
		// Reserved: injected by the template engine
		"existing": true,
		// Internal context keys from tags.go
		contextKeyResolver:     true,
		contextKeyIncludeStack: true,
		contextKeyTemplateSet:  true,
		contextKeyVariables:    true,
		// pongo2 internal
		pongo2MetaContextKey: true,
	}
}()

// filterKnownUndefined removes internally-injected context keys from the
// list of undefined variables, returning only genuinely user-referenced
// variables that were not defined.
func filterKnownUndefined(vars []string) []string {
	var result []string
	for _, v := range vars {
		if !knownUndefined[v] {
			result = append(result, v)
		}
	}
	return result
}

// isReservedVarName checks if a variable name is reserved by tai.
// Reserved names are: "existing" and any name with the "_tai_" prefix.
func isReservedVarName(name string) bool {
	if name == "existing" {
		return true
	}
	return strings.HasPrefix(name, "_tai_")
}

// ValidateVarNames checks that user-provided variable names don't collide
// with reserved names. Returns an error listing any conflicts.
func ValidateVarNames(vars Variables) error {
	var conflicts []string
	for k := range vars {
		if isReservedVarName(k) {
			conflicts = append(conflicts, k)
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("reserved variable name(s) cannot be used: %v", conflicts)
	}
	return nil
}

// readExistingContent reads the content of an existing file.
func readExistingContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// injectTagContext sets the pongo2 context keys required by custom tags
// (currently include_asset). Centralizing this ensures Process() and
// tagIncludeAsset.Execute() stay in sync on which keys exist.
func injectTagContext(ctx pongo2.Context, resolver ContentResolver, set *pongo2.TemplateSet, vars Variables) {
	ctx[contextKeyResolver] = resolver
	ctx[contextKeyIncludeStack] = []string{}
	ctx[contextKeyTemplateSet] = set
	ctx[contextKeyVariables] = vars
}

// templateSyntaxPattern matches any pongo2 template syntax: {{ }}, {% %}, or {# #}.
var templateSyntaxPattern = regexp.MustCompile(`\{\{|\{%|\{#`)

// HasTemplateSyntax checks if content contains any template syntax.
// This detects {{ variables }}, {% tags %}, and {# comments #}.
func HasTemplateSyntax(content []byte) bool {
	return templateSyntaxPattern.Match(content)
}

// existingPattern matches both {{ existing }} and {{existing}}.
var existingPattern = regexp.MustCompile(`\{\{\s*existing\s*\}\}`)

// HasExistingVar checks if content contains the {{ existing }} variable.
func HasExistingVar(content []byte) bool {
	return existingPattern.Match(content)
}

// Deprecated: HasTemplateVars is an alias for HasTemplateSyntax, kept for
// backward compatibility. Use HasTemplateSyntax instead. Will be removed
// when all internal callers have been migrated.
func HasTemplateVars(content []byte) bool {
	return HasTemplateSyntax(content)
}
