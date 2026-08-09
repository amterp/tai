package configasset

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/amterp/tai/internal/config"
)

func init() {
	Register(&mcpHandler{})
}

// MCPTranslator is implemented by adapters that support MCP servers.
//
// Unlike hooks, whose three-level shape is identical everywhere, MCP entries
// differ structurally between targets: OpenCode wants the command and its args
// as one argv array and spells the environment `environment`, while the others
// keep them separate and call it `env`. So each adapter builds its own entry
// rather than tweaking a shared one.
type MCPTranslator interface {
	// TransformMCP renders an MCP server into this target's shape. Warnings
	// report fields this target cannot express, which are dropped rather than
	// failing the install; a transport the target does not support is an error.
	TransformMCP(name string, cfg *config.MCPConfig) (entry map[string]any, warnings []string, err error)
}

// mcpHandler installs MCP servers as keyed entries in a target's server map.
type mcpHandler struct{}

func (h *mcpHandler) Type() string { return config.AssetTypeMCP }

// Cardinality is Collection: a profile carries as many servers as it likes.
func (h *mcpHandler) Cardinality() Cardinality { return Collection }

func (h *mcpHandler) Plan(req Request) (*Plan, error) {
	cfg, err := config.ParseMCPConfig(req.Content)
	if err != nil {
		return nil, err
	}
	if err := ValidateMCPConfig(cfg); err != nil {
		return nil, fmt.Errorf("validating mcp server %s: %w", req.Name, err)
	}

	plan := &Plan{}

	// A disabled server is omitted rather than installed-and-switched-off:
	// only some targets have a per-server toggle, and "absent" means the same
	// thing on all of them.
	if !cfg.IsEnabled() {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("mcp server %s is disabled - not installed", req.Name))
		return plan, nil
	}

	translator, ok := req.Adapter.(MCPTranslator)
	if !ok {
		return nil, fmt.Errorf("target %q does not support MCP servers", req.Adapter.Name())
	}

	entry, warnings, err := translator.TransformMCP(req.Name, cfg)
	if err != nil {
		return nil, fmt.Errorf("mcp server %s: %w", req.Name, err)
	}
	plan.Warnings = append(plan.Warnings, warnings...)

	file := req.Adapter.ConfigFilePath(config.AssetTypeMCP, req.Scope)
	if file == "" {
		return nil, fmt.Errorf("target %q has no MCP config file for scope %q",
			req.Adapter.Name(), req.Scope)
	}

	key := req.Adapter.SettingsKey(config.AssetTypeMCP)
	if key == "" {
		return nil, fmt.Errorf("target %q has no settings key for MCP servers", req.Adapter.Name())
	}

	if consenter, ok := req.Adapter.(ConfigWriteConsenter); ok {
		plan.Consent = consenter.ConfigWriteConsent(config.AssetTypeMCP, file)
	}

	plan.Edits = append(plan.Edits, Edit{
		File:    file,
		KeyPath: []string{key},
		Mode:    UpsertMapEntry,
		ID:      req.Name,
		Value:   entry,
	})

	return plan, nil
}

// ValidateMCPConfig checks an MCP server definition.
func ValidateMCPConfig(cfg *config.MCPConfig) error {
	if cfg == nil {
		return fmt.Errorf("empty mcp config")
	}

	switch cfg.Transport {
	case "":
		return fmt.Errorf("'transport' is required; must be one of: %s",
			strings.Join(config.ValidMCPTransports(), ", "))

	case config.MCPTransportStdio:
		if strings.TrimSpace(cfg.Command) == "" {
			return fmt.Errorf("'command' is required for stdio transport")
		}
		if cfg.URL != "" {
			return fmt.Errorf("'url' is not valid for stdio transport")
		}
		if len(cfg.Headers) > 0 {
			return fmt.Errorf("'headers' is not valid for stdio transport")
		}

	case config.MCPTransportHTTP:
		if strings.TrimSpace(cfg.URL) == "" {
			return fmt.Errorf("'url' is required for http transport")
		}
		if cfg.Command != "" {
			return fmt.Errorf("'command' is not valid for http transport")
		}
		if len(cfg.Args) > 0 {
			return fmt.Errorf("'args' is not valid for http transport")
		}
		if len(cfg.Env) > 0 {
			return fmt.Errorf("'env' is not valid for http transport; use 'headers'")
		}
		if cfg.Cwd != "" {
			return fmt.Errorf("'cwd' is not valid for http transport")
		}

	default:
		return fmt.Errorf("unknown transport %q; must be one of: %s",
			cfg.Transport, strings.Join(config.ValidMCPTransports(), ", "))
	}

	return nil
}

// envRefPattern matches a ${VAR} reference.
var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// WholeEnvRef returns the variable name when s is exactly one ${VAR}
// reference and nothing else.
//
// The distinction matters because targets resolve references at different
// layers. Most interpolate anywhere in a string, but some take an environment
// variable *name* in a dedicated field, which can only express a whole-value
// reference. Those targets can translate "${TOKEN}" and cannot translate
// "Bearer ${TOKEN}".
func WholeEnvRef(s string) (string, bool) {
	m := envRefPattern.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	if m[0] != s {
		return "", false
	}
	return m[1], true
}

// HasEnvRef reports whether s contains any ${VAR} reference.
func HasEnvRef(s string) bool {
	return envRefPattern.MatchString(s)
}

// RewriteEnvRefs replaces every ${VAR} with the result of render(VAR), for
// targets that use a different interpolation syntax.
func RewriteEnvRefs(s string, render func(name string) string) string {
	return envRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := envRefPattern.FindStringSubmatch(match)[1]
		return render(name)
	})
}
