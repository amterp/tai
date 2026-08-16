// Package codex provides the adapter for Codex.
package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
	fm "github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/template"
)

func init() {
	adapter.DefaultRegistry.Register(New(""))
}

// Adapter implements the adapter.Adapter interface for Codex.
type Adapter struct {
	targetDir string // Override for testing
}

// HooksSettingsKey is the top-level key Codex reads hooks from.
const HooksSettingsKey = "hooks"

// MCPSettingsKey is the TOML table Codex reads MCP servers from. Note the
// snake_case spelling, unlike the mcpServers other targets use.
const MCPSettingsKey = "mcp_servers"

// mcpConsentKey identifies the "may tai rewrite config.toml" decision, so the
// answer can be remembered across runs.
const mcpConsentKey = "codex:config.toml"

// hookEvents maps canonical tai events to Codex's names.
//
// Codex's hooks are modelled closely on Claude's, down to the field names, so
// most events keep the same spelling. session_end is deliberately absent: it
// appears in Codex's docs but not in the binary's event enum, and a hook wired
// to an event the target does not dispatch fails silently. Leaving it out
// means tai says so instead.
var hookEvents = map[string]string{
	config.HookEventSessionStart:      "SessionStart",
	config.HookEventUserPromptSubmit:  "UserPromptSubmit",
	config.HookEventPreToolUse:        "PreToolUse",
	config.HookEventPostToolUse:       "PostToolUse",
	config.HookEventStop:              "Stop",
	config.HookEventSubagentStart:     "SubagentStart",
	config.HookEventSubagentStop:      "SubagentStop",
	config.HookEventPreCompact:        "PreCompact",
	config.HookEventPostCompact:       "PostCompact",
	config.HookEventPermissionRequest: "PermissionRequest",
}

// hookTools maps canonical tai tool names to Codex's.
//
// Only the tools whose Codex names are established are mapped. An unmapped
// tool is an error pointing at the per-target `match` escape hatch, which is
// the safe failure: a matcher silently missing a tool name would widen to
// every tool, so a hook meant to guard edits would fire on everything.
var hookTools = map[string]string{
	config.HookToolShell: "Bash",
	config.HookToolEdit:  "apply_patch",
}

// New creates a new Codex adapter.
// If targetDir is empty, defaults to $CODEX_HOME or ~/.codex.
func New(targetDir string) *Adapter {
	if targetDir == "" {
		targetDir = os.Getenv("CODEX_HOME")
	}
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			targetDir = ".codex" // Fallback
		} else {
			targetDir = filepath.Join(home, ".codex")
		}
	}
	return &Adapter{targetDir: targetDir}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "codex"
}

// AssetMapping returns how tai asset types map to Codex paths.
// Codex has no native persona concept, so personas are mapped to skills.
func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypePersona:  "skills",
		config.AssetTypeDelegate: "delegates",
		config.AssetTypeSkill:    "skills",
		// Note: "file" type uses asset.Dest directly, not this mapping
	}
}

// TargetDir returns the Codex config directory.
func (a *Adapter) TargetDir() string {
	return a.targetDir
}

// MapAssetPath maps a tai asset to a Codex target path.
// Single-file assets that map to skills/ are wrapped in a directory
// (skills/name/SKILL.md) because Codex only discovers directory-format skills.
func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	if !asset.IsDir && a.mapsToSkills(asset.Type) {
		name := strings.TrimSuffix(asset.Name, ".md")
		return filepath.Join("skills", name, "SKILL.md")
	}
	return adapter.DefaultMapAssetPath(a.AssetMapping(), asset)
}

// mapsToSkills returns true if the given asset type maps to the skills/ directory.
func (a *Adapter) mapsToSkills(assetType string) bool {
	return a.AssetMapping()[assetType] == "skills"
}

// TransformContent transforms asset content into Codex-compatible format.
// Personas are mapped to skills, so they get the same frontmatter (name + description).
// Files are returned as-is. Other types get frontmatter re-rendered from tai metadata.
func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	switch asset.Type {
	case config.AssetTypePersona, config.AssetTypeSkill:
		return a.transformSkill(asset), nil
	case config.AssetTypeDelegate:
		return a.transformDelegate(asset), nil
	case config.AssetTypeFile:
		return asset.Content, nil
	default:
		return asset.Content, nil
	}
}

func (a *Adapter) transformDelegate(asset *config.ResolvedAsset) []byte {
	var fields fm.Fields
	fields.Set(fm.FieldName, asset.Metadata[fm.FieldName])
	fields.SetString(fm.FieldDescription, asset.Metadata[fm.FieldDescription])
	fields.SetString(fm.FieldTools, asset.Metadata[fm.FieldTools])
	fields.SetString(fm.FieldModel, asset.Metadata[fm.FieldModel])
	fields.Merge(asset.Native[a.Name()])

	return fm.Render(fields, asset.Content)
}

func (a *Adapter) transformSkill(asset *config.ResolvedAsset) []byte {
	var fields fm.Fields
	fields.Set(fm.FieldName, asset.Metadata[fm.FieldName])
	fields.SetString(fm.FieldDescription, asset.Metadata[fm.FieldDescription])
	fields.Merge(asset.Native[a.Name()])

	return fm.Render(fields, asset.Content)
}

// Run invokes codex with the given arguments.
func (a *Adapter) Run(args []string) error {
	return adapter.RunBinary("codex", args)
}

// SupportedConfigAssetTypes returns the config asset types Codex supports.
func (a *Adapter) SupportedConfigAssetTypes() []string {
	return []string{config.AssetTypeHook, config.AssetTypeMCP}
}

// ConfigWriteConsent asks before tai rewrites config.toml.
//
// Codex has no dedicated MCP file, so servers land in the same config.toml
// that holds the user's model settings, notify command, and per-project trust
// levels. tai has to reparse and re-render that whole file to add one table,
// and no Go TOML library preserves comments or key order - so the file comes
// back correct but reformatted. That is a real cost to a file tai does not
// own, and worth one question rather than a surprise.
func (a *Adapter) ConfigWriteConsent(assetType, file string) *configasset.ConsentRequest {
	if assetType != config.AssetTypeMCP {
		return nil
	}
	return &configasset.ConsentRequest{
		Key:      mcpConsentKey,
		Question: "Let tai manage MCP servers in Codex's config.toml?",
		Details: []string{
			"Codex has no separate MCP config file, so servers go in " + file + ",",
			"alongside your model settings, notify command, and project trust levels.",
			"",
			"tai reads that file, adds its servers, and writes it back. Your settings",
			"are preserved, but comments and key order are not - the file comes back",
			"reformatted. tai only ever removes the servers it added.",
		},
	}
}

// TransformMCP renders an MCP server in Codex's shape.
//
// Codex does no string interpolation at all. Instead it takes environment
// variable *names* in dedicated fields, which can express a whole-value
// reference and nothing else. So "${TOKEN}" translates cleanly while
// "Bearer ${TOKEN}" cannot, and saying so beats installing a server that
// authenticates with a literal dollar sign.
// MCPKeyPath locates the server table. Codex has one config file and one
// [mcp_servers] table in it, whatever the scope.
func (a *Adapter) MCPKeyPath(_ string) []string {
	return []string{MCPSettingsKey}
}

func (a *Adapter) TransformMCP(_ string, cfg *config.MCPConfig) (map[string]any, []string, error) {
	entry := map[string]any{}
	var warnings []string

	switch cfg.Transport {
	case config.MCPTransportStdio:
		entry["command"] = cfg.Command
		if len(cfg.Args) > 0 {
			entry["args"] = configasset.AnySlice(cfg.Args)
		}
		if cfg.Cwd != "" {
			entry["cwd"] = cfg.Cwd
		}

		env := map[string]any{}
		var forward []any
		for key, value := range cfg.Env {
			if !configasset.HasEnvRef(value) {
				env[key] = value
				continue
			}
			name, whole := configasset.WholeEnvRef(value)
			if !whole {
				return nil, nil, fmt.Errorf(
					"env %q embeds an environment reference in a larger string, which Codex cannot express; "+
						"use a whole-value reference like \"${VAR}\", or set it via native.codex", key)
			}
			if name != key {
				return nil, nil, fmt.Errorf(
					"env %q reads ${%s}: Codex can only forward a variable under its own name; "+
						"rename the key to %s, or set it via native.codex", key, name, name)
			}
			// Codex forwards a variable of the same name rather than assigning
			// a value, which is exactly what a whole-value self-reference means.
			forward = append(forward, name)
		}
		if len(env) > 0 {
			entry["env"] = env
		}
		if len(forward) > 0 {
			sortAny(forward)
			entry["env_vars"] = forward
		}

	case config.MCPTransportHTTP:
		entry["url"] = cfg.URL

		headers := map[string]any{}
		envHeaders := map[string]any{}
		for key, value := range cfg.Headers {
			if !configasset.HasEnvRef(value) {
				headers[key] = value
				continue
			}
			if name, ok := bearerEnvRef(key, value); ok {
				// The overwhelmingly common shape, and the one Codex has a
				// dedicated field for.
				entry["bearer_token_env_var"] = name
				continue
			}
			name, whole := configasset.WholeEnvRef(value)
			if !whole {
				return nil, nil, fmt.Errorf(
					"header %q embeds an environment reference in a larger string, which Codex cannot express; "+
						"use a whole-value reference, an Authorization: \"Bearer ${VAR}\" header, or native.codex", key)
			}
			envHeaders[key] = name
		}
		if len(headers) > 0 {
			entry["http_headers"] = headers
		}
		if len(envHeaders) > 0 {
			entry["env_http_headers"] = envHeaders
		}
	}

	if cfg.Description != "" {
		warnings = append(warnings, "Codex MCP entries have no 'description' field - dropped")
	}

	for key, value := range cfg.Native[a.Name()] {
		entry[key] = value
	}

	return entry, warnings, nil
}

// bearerEnvRef recognizes an Authorization header of the form
// "Bearer ${VAR}", which Codex expresses with bearer_token_env_var.
func bearerEnvRef(header, value string) (string, bool) {
	if !strings.EqualFold(header, "Authorization") {
		return "", false
	}
	rest, ok := strings.CutPrefix(value, "Bearer ")
	if !ok {
		return "", false
	}
	return configasset.WholeEnvRef(rest)
}

// sortAny sorts a slice of strings held as any, for deterministic output.
func sortAny(in []any) {
	sort.Slice(in, func(i, j int) bool {
		return in[i].(string) < in[j].(string)
	})
}

// InstallWarnings returns Codex-specific advisory install warnings.
func (a *Adapter) InstallWarnings(asset *config.Asset) []string {
	switch asset.Type {
	case config.AssetTypePersona:
		return []string{
			fmt.Sprintf("Codex does not support personas - installed as skill instead: skills/%s/SKILL.md",
				strings.TrimSuffix(asset.Name, ".md")),
		}
	case config.AssetTypeDelegate:
		return []string{
			fmt.Sprintf("Codex does not natively consume delegate assets - installed as passive reference file: delegates/%s", asset.Name),
		}
	case config.AssetTypeHook:
		// Codex will not run a hook until it is trusted, and trust is keyed on
		// a hash of the definition - so every reinstall re-arms the prompt.
		// Writing the file is necessary but not sufficient, and staying quiet
		// about it looks exactly like tai not working.
		return []string{
			"Codex requires hooks to be trusted before they run, and every change re-arms that - run /hooks in Codex to review them",
		}
	default:
		return nil
	}
}

// ConfigFilePath returns the Codex config file for an asset type and scope.
//
// Hooks get a dedicated hooks.json rather than going into config.toml. Codex
// reads both, and the JSON file means tai never has to rewrite the TOML that
// also holds the user's model settings and project trust decisions.
func (a *Adapter) ConfigFilePath(assetType, scope string) string {
	var name string
	switch assetType {
	case config.AssetTypeHook:
		name = "hooks.json"
	case config.AssetTypeMCP:
		// No dedicated MCP file exists; servers live in the main config.
		name = "config.toml"
	default:
		return ""
	}

	switch scope {
	case config.ScopeProject:
		rel := filepath.Join(".codex", name)
		abs, err := filepath.Abs(rel)
		if err != nil {
			return rel
		}
		return abs
	case config.ScopeUser:
		return filepath.Join(a.targetDir, name)
	default:
		// Codex has no local scope.
		return ""
	}
}

// HookEventName maps a canonical tai event to Codex's name for it.
func (a *Adapter) HookEventName(canonical string) (string, bool) {
	name, ok := hookEvents[canonical]
	return name, ok
}

// HookToolName maps a canonical tai tool name to Codex's name for it.
func (a *Adapter) HookToolName(canonical string) (string, bool) {
	name, ok := hookTools[canonical]
	return name, ok
}

// HookTimeoutUnit reports that Codex expects hook timeouts in seconds.
func (a *Adapter) HookTimeoutUnit() configasset.TimeoutUnit {
	return configasset.TimeoutSeconds
}

// SettingsKey returns the JSON key name for a config asset type.
func (a *Adapter) SettingsKey(assetType string) string {
	switch assetType {
	case config.AssetTypeHook:
		return HooksSettingsKey
	case config.AssetTypeMCP:
		return MCPSettingsKey
	default:
		return ""
	}
}

// MainFileName returns the main file name for a directory asset type.
func (a *Adapter) MainFileName(assetType string) string {
	return config.MainFileName(assetType)
}

// UnsupportedFileAssetTypes returns nil - Codex installs all file asset types
// (though some are passive reference only, see InstallWarnings).
func (a *Adapter) UnsupportedFileAssetTypes() []string {
	return nil
}

// ProvideVariables returns Codex-specific template variables.
func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         "codex",
		"target_name_display": "Codex",
		"target_agents_file":  "AGENTS.md",
	}
}
