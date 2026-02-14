package template

import (
	"bytes"
	"fmt"
	"strings"

	pongo2 "github.com/flosch/pongo2/v6"
)

// Context keys for passing tai-specific state through pongo2 execution.
const (
	contextKeyResolver     = "_tai_resolver"
	contextKeyIncludeStack = "_tai_include_stack"
	contextKeyTemplateSet  = "_tai_template_set"
	contextKeyVariables    = "_tai_variables"
)

// maxIncludeDepth limits how deep {% include_asset %} nesting can go.
// This guards against pathological (non-circular) chains of includes.
const maxIncludeDepth = 50

func init() {
	// Register our custom include_asset tag globally.
	// This is safe because the tag retrieves its resolver from the execution
	// context, so it's a no-op when no resolver is provided.
	if err := pongo2.RegisterTag("include_asset", tagIncludeAssetParser); err != nil {
		panic(fmt.Sprintf("failed to register include_asset tag: %v", err))
	}
}

// tagIncludeAsset is the node for {% include_asset "ref" %}.
type tagIncludeAsset struct {
	ref string // Asset reference, e.g. "local:personas/senior-swe.md"
}

// tagIncludeAssetParser parses {% include_asset "ref" %}.
func tagIncludeAssetParser(_ *pongo2.Parser, _ *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	token := arguments.MatchType(pongo2.TokenString)
	if token == nil {
		return nil, arguments.Error("include_asset requires a quoted string argument", nil)
	}

	// Ensure no extra arguments
	if arguments.Remaining() > 0 {
		return nil, arguments.Error("include_asset takes exactly one argument", nil)
	}

	ref := token.Val

	// Validate the reference at parse time for clear, early errors
	if ref == "" {
		return nil, arguments.Error("include_asset: asset reference cannot be empty", nil)
	}
	if strings.HasPrefix(ref, "/") {
		return nil, arguments.Error("include_asset: absolute paths are not allowed; use a relative asset path like \"personas/foo.md\"", nil)
	}

	return &tagIncludeAsset{ref: ref}, nil
}

// Execute resolves the asset, processes its template content recursively,
// and writes the result to the output.
func (t *tagIncludeAsset) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	// Retrieve resolver from context
	resolverVal, ok := ctx.Public[contextKeyResolver]
	if !ok || resolverVal == nil {
		return ctx.Error("include_asset requires a ContentResolver (not available in this context)", nil)
	}
	resolver, ok := resolverVal.(ContentResolver)
	if !ok {
		return ctx.Error("include_asset: invalid resolver type in context", nil)
	}

	// Retrieve include stack for cycle detection
	stackVal, ok := ctx.Public[contextKeyIncludeStack]
	if !ok {
		return ctx.Error("include_asset: missing include stack in context", nil)
	}
	stack, ok := stackVal.([]string)
	if !ok {
		return ctx.Error("include_asset: invalid include stack type", nil)
	}

	// Check include depth limit
	if len(stack) >= maxIncludeDepth {
		return ctx.Error(fmt.Sprintf("include_asset: maximum nesting depth of %d exceeded", maxIncludeDepth), nil)
	}

	// Normalize the ref so "personas/a.md" and "local:personas/a.md"
	// are treated as the same asset for cycle detection purposes.
	normalizedRef := normalizeRef(t.ref)

	// Cycle detection using normalized refs
	for _, entry := range stack {
		if entry == normalizedRef {
			chain := append(stack, normalizedRef)
			return ctx.Error(fmt.Sprintf("include_asset cycle detected: %s", strings.Join(chain, " -> ")), nil)
		}
	}

	// Resolve asset content (frontmatter stripped)
	content, err := resolver.ResolveAssetContent(t.ref)
	if err != nil {
		return ctx.Error(fmt.Sprintf("include_asset %q: %v", t.ref, err), nil)
	}

	// If the included content has no template syntax, output it directly
	if !HasTemplateSyntax(content) {
		_, writeErr := writer.WriteString(string(content))
		if writeErr != nil {
			return ctx.Error(fmt.Sprintf("include_asset %q: write error: %v", t.ref, writeErr), nil)
		}
		return nil
	}

	// Retrieve template set for recursive processing
	setVal, ok := ctx.Public[contextKeyTemplateSet]
	if !ok {
		return ctx.Error("include_asset: missing template set in context", nil)
	}
	set, ok := setVal.(*pongo2.TemplateSet)
	if !ok {
		return ctx.Error("include_asset: invalid template set type", nil)
	}

	// Parse the included content as a new template
	tpl, tplErr := set.FromBytes(content)
	if tplErr != nil {
		return ctx.Error(fmt.Sprintf("include_asset %q: parsing included content: %v", t.ref, tplErr), nil)
	}

	// Build new context with updated include stack
	newCtx := pongo2.Context{}
	for k, v := range ctx.Public {
		newCtx[k] = v
	}
	newStack := make([]string, len(stack)+1)
	copy(newStack, stack)
	newStack[len(stack)] = normalizedRef
	newCtx[contextKeyIncludeStack] = newStack

	// Execute recursively
	var buf bytes.Buffer
	if execErr := tpl.ExecuteWriter(newCtx, &buf); execErr != nil {
		return ctx.Error(fmt.Sprintf("include_asset %q: executing included content: %v", t.ref, execErr), nil)
	}

	_, writeErr := writer.WriteString(buf.String())
	if writeErr != nil {
		return ctx.Error(fmt.Sprintf("include_asset %q: write error: %v", t.ref, writeErr), nil)
	}

	return nil
}

// normalizeRef ensures asset references have a consistent form for comparison.
// Refs without a source prefix get "local:" prepended, matching ParseAssetReference behavior.
func normalizeRef(ref string) string {
	if strings.Contains(ref, ":") {
		return ref
	}
	return "local:" + ref
}
