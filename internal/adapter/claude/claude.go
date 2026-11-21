// Package claude provides the adapter for Claude Code.
package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
	fm "github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/template"
)

// Settings key constants for Claude's settings.json
const (
	StatusLineSettingsKey = "statusLine"
	HooksSettingsKey      = "hooks"
	MCPSettingsKey        = "mcpServers"

	// ProjectsSettingsKey holds ~/.claude.json's per-project records, keyed by
	// absolute path. Project-local MCP servers live inside them.
	ProjectsSettingsKey = "projects"
)

// hookEvents maps canonical tai events to Claude's names. Claude has more
// events than tai abstracts; the ones here are those with an equivalent on at
// least one other target, which is what makes them worth a portable name.
var hookEvents = map[string]string{
	config.HookEventSessionStart:      "SessionStart",
	config.HookEventSessionEnd:        "SessionEnd",
	config.HookEventUserPromptSubmit:  "UserPromptSubmit",
	config.HookEventPreToolUse:        "PreToolUse",
	config.HookEventPostToolUse:       "PostToolUse",
	config.HookEventStop:              "Stop",
	config.HookEventSubagentStart:     "SubagentStart",
	config.HookEventSubagentStop:      "SubagentStop",
	config.HookEventPreCompact:        "PreCompact",
	config.HookEventPostCompact:       "PostCompact",
	config.HookEventNotification:      "Notification",
	config.HookEventPermissionRequest: "PermissionRequest",
}

// hookTools maps canonical tai tool names to Claude's.
var hookTools = map[string]string{
	config.HookToolRead:  "Read",
	config.HookToolWrite: "Write",
	config.HookToolEdit:  "Edit",
	config.HookToolShell: "Bash",
}

func init() {
	adapter.DefaultRegistry.Register(New(""))
}

// Adapter implements the adapter.Adapter interface for Claude Code.
type Adapter struct {
	targetDir string // Override for testing
}

// New creates a new Claude adapter.
// If targetDir is empty, defaults to ~/.claude
func New(targetDir string) *Adapter {
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			targetDir = ".claude" // Fallback
		} else {
			targetDir = filepath.Join(home, ".claude")
		}
	}
	return &Adapter{targetDir: targetDir}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "claude"
}

// AssetMapping returns how tai asset types map to Claude paths.
func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypePersona:  "output-styles",
		config.AssetTypeDelegate: "agents",
		config.AssetTypeSkill:    "skills",
		// Note: "file" type uses asset.Dest directly, not this mapping
	}
}

// TargetDir returns the Claude config directory.
func (a *Adapter) TargetDir() string {
	return a.targetDir
}

// MapAssetPath maps a tai asset to a Claude target path.
func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	return adapter.DefaultMapAssetPath(a.AssetMapping(), asset)
}

// TransformContent transforms asset content into Claude's expected format.
// Personas get name + description frontmatter.
// Delegates and skills get name + description + tools frontmatter.
// Files are returned as-is (no frontmatter processing).
func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	switch asset.Type {
	case config.AssetTypePersona:
		return a.transformPersona(asset), nil
	case config.AssetTypeDelegate, config.AssetTypeSkill:
		return a.transformAgent(asset), nil
	case config.AssetTypeFile:
		// Files are copied as-is, no transformation
		return asset.Content, nil
	default:
		// Unknown types: return content as-is
		return asset.Content, nil
	}
}

// transformPersona renders Claude output-style frontmatter (name, description).
func (a *Adapter) transformPersona(asset *config.ResolvedAsset) []byte {
	var fields fm.Fields
	fields.Set(fm.FieldName, asset.Metadata[fm.FieldName])
	fields.SetString(fm.FieldDescription, asset.Metadata[fm.FieldDescription])
	fields.Merge(asset.Native[a.Name()])

	return fm.Render(fields, asset.Content)
}

// transformAgent renders Claude agent/skill frontmatter (name, description, tools).
func (a *Adapter) transformAgent(asset *config.ResolvedAsset) []byte {
	var fields fm.Fields
	fields.Set(fm.FieldName, asset.Metadata[fm.FieldName])
	fields.SetString(fm.FieldDescription, asset.Metadata[fm.FieldDescription])
	fields.SetString(fm.FieldTools, asset.Metadata[fm.FieldTools])
	fields.SetString(fm.FieldModel, asset.Metadata[fm.FieldModel])
	fields.Merge(asset.Native[a.Name()])

	return fm.Render(fields, asset.Content)
}

// Run invokes claude with the given arguments.
func (a *Adapter) Run(args []string) error {
	return adapter.RunBinary("claude", args)
}

// SupportedConfigAssetTypes returns the config asset types Claude supports.
func (a *Adapter) SupportedConfigAssetTypes() []string {
	return []string{
		config.AssetTypeStatusLine, config.AssetTypeHook, config.AssetTypeMCP,
		config.AssetTypeSettings, config.AssetTypeApproval,
	}
}

// TransformMCP renders an MCP server in Claude's shape.
//
// Claude interpolates ${VAR} itself when it reads the config, so environment
// references are passed through untouched.
func (a *Adapter) TransformMCP(_ string, cfg *config.MCPConfig) (map[string]any, []string, error) {
	entry := map[string]any{}
	var warnings []string

	switch cfg.Transport {
	case config.MCPTransportStdio:
		entry["type"] = "stdio"
		entry["command"] = cfg.Command
		if len(cfg.Args) > 0 {
			entry["args"] = configasset.AnySlice(cfg.Args)
		}
		if len(cfg.Env) > 0 {
			entry["env"] = configasset.AnyMap(cfg.Env)
		}
		if cfg.Cwd != "" {
			// Claude has no per-server working directory. Dropping it is
			// better than failing, but saying so matters: a server relying on
			// it will start in the wrong place.
			warnings = append(warnings,
				"Claude has no per-server 'cwd' for MCP servers - dropped; make the command's own path absolute instead")
		}

	case config.MCPTransportHTTP:
		entry["type"] = "http"
		entry["url"] = cfg.URL
		if len(cfg.Headers) > 0 {
			entry["headers"] = configasset.AnyMap(cfg.Headers)
		}
	}

	for key, value := range cfg.Native[a.Name()] {
		entry[key] = value
	}

	return entry, warnings, nil
}

// HookEventName maps a canonical tai event to Claude's name for it.
func (a *Adapter) HookEventName(canonical string) (string, bool) {
	name, ok := hookEvents[canonical]
	return name, ok
}

// HookToolName maps a canonical tai tool name to Claude's name for it.
func (a *Adapter) HookToolName(canonical string) (string, bool) {
	name, ok := hookTools[canonical]
	return name, ok
}

// HookTimeoutUnit reports that Claude expects hook timeouts in seconds.
func (a *Adapter) HookTimeoutUnit() configasset.TimeoutUnit {
	return configasset.TimeoutSeconds
}

func (a *Adapter) HookEdit(scope string, hook configasset.TranslatedHook) (configasset.Edit, error) {
	return configasset.GroupedHookEdit(a, scope, hook)
}

// InstallWarnings returns Claude-specific advisory install warnings.
func (a *Adapter) InstallWarnings(_ *config.Asset) []string {
	return nil
}

// ConfigFilePath returns the Claude config file a config asset type installs
// into at the given scope.
//
// Project and local scopes resolve against the working directory, which is
// what "this project" means for a CLI invoked inside a repo. They are returned
// absolute so nothing downstream has to guess the base.
func (a *Adapter) ConfigFilePath(assetType, scope string) string {
	switch assetType {
	case config.AssetTypeStatusLine, config.AssetTypeHook, config.AssetTypeSettings:
		return a.settingsFilePath(scope)
	case config.AssetTypeMCP:
		return a.mcpFilePath(scope)
	case config.AssetTypeApproval:
		// The auto mode classifier ignores autoMode in project and local
		// settings, so those scopes have nowhere to put a policy.
		if scope != config.ScopeUser {
			return ""
		}
		return a.settingsFilePath(scope)
	default:
		return ""
	}
}

// ApprovalKeyPath locates the auto mode array holding entries of kind.
func (a *Adapter) ApprovalKeyPath(kind string) []string {
	switch kind {
	case config.ApprovalContext:
		return []string{"autoMode", "environment"}
	case config.ApprovalAllow:
		return []string{"autoMode", "allow"}
	case config.ApprovalDeny:
		return []string{"autoMode", "soft_deny"}
	default:
		return nil
	}
}

// ApprovalArraySeed is "$defaults": an auto mode array without it replaces
// Claude's built-in rules instead of adding to them, so a soft_deny array
// holding only tai's entries would quietly drop every default block rule.
func (a *Adapter) ApprovalArraySeed() any { return "$defaults" }

// mcpFilePath locates the file Claude reads MCP servers from.
//
// MCP servers do not live in settings.json. Project scope gets .mcp.json, a
// dedicated single-key file meant to be committed. User and local scope both
// land in ~/.claude.json alongside a session's entire mutable state - account
// identity, per-project trust decisions, caches - which is why every write
// merges into the parsed document rather than rendering the file. They differ
// by where in that file they sit, not by which file: see mcpKeyPath.
func (a *Adapter) mcpFilePath(scope string) string {
	if scope == config.ScopeProject {
		return projectPath(".mcp.json")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

// MCPKeyPath locates the server map for a scope.
//
// Local scope is the odd one. Claude records a private, uncommitted server
// under the project's own entry in ~/.claude.json - keyed by absolute path,
// beside that project's trust decision and history - rather than in any file in
// the project directory. Writing a .claude.json in the repo instead would
// produce a file Claude never reads, so the server would simply never appear.
func (a *Adapter) MCPKeyPath(scope string) []string {
	if scope == config.ScopeLocal {
		return []string{ProjectsSettingsKey, projectPath("."), MCPSettingsKey}
	}
	return []string{MCPSettingsKey}
}

// settingsFilePath locates settings.json for a scope. Config asset types that
// live in settings.json share it.
func (a *Adapter) settingsFilePath(scope string) string {
	switch scope {
	case config.ScopeProject:
		return projectPath(".claude", "settings.json")
	case config.ScopeLocal:
		return projectPath(".claude", "settings.local.json")
	default: // user
		return filepath.Join(a.targetDir, "settings.json")
	}
}

// projectPath builds an absolute path under the working directory. A relative
// path would be re-resolved against whatever directory later code happened to
// run in; resolving once here keeps the manifest's record of what was written
// meaningful.
func projectPath(parts ...string) string {
	rel := filepath.Join(parts...)
	abs, err := filepath.Abs(rel)
	if err != nil {
		return rel
	}
	return abs
}

// TransformStatusLine transforms a tai status line config to Claude's format.
func (a *Adapter) TransformStatusLine(cfg *config.StatusLineConfig, rawNative []byte) (map[string]any, error) {
	// If native format, parse and return as-is
	if rawNative != nil {
		var native map[string]any
		if err := json.Unmarshal(rawNative, &native); err != nil {
			// Try YAML if JSON fails
			return nil, fmt.Errorf("native status line config is not valid JSON or YAML: %w", err)
		}
		return native, nil
	}

	// Transform tai format to Claude format
	result := make(map[string]any)

	switch cfg.Level {
	case config.StatusLineLevelCustom:
		result["type"] = "command"
		result["command"] = cfg.Command
		if cfg.Padding != nil {
			result["padding"] = *cfg.Padding
		}
	case config.StatusLineLevelHidden:
		result["style"] = "hidden"
	case config.StatusLineLevelMinimal:
		result["style"] = "minimal"
		a.applyShowElements(result, cfg.Show)
	case config.StatusLineLevelStandard:
		result["style"] = "standard"
		a.applyShowElements(result, cfg.Show)
	case config.StatusLineLevelDetailed:
		result["style"] = "detailed"
		a.applyShowElements(result, cfg.Show)
	default:
		return nil, fmt.Errorf("unknown status line level: %s", cfg.Level)
	}

	return result, nil
}

// SettingsKey returns the JSON key name for a config asset type in Claude's settings.json.
func (a *Adapter) SettingsKey(assetType string) string {
	switch assetType {
	case config.AssetTypeStatusLine:
		return StatusLineSettingsKey
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

// UnsupportedFileAssetTypes returns nil - Claude supports all file asset types.
func (a *Adapter) UnsupportedFileAssetTypes() []string {
	return nil
}

// ProvideVariables returns Claude-specific template variables.
func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         "claude",
		"target_name_display": "Claude",
		"target_agents_file":  "CLAUDE.md",
	}
}

// applyShowElements sets Claude's show* flags based on tai's show elements.
func (a *Adapter) applyShowElements(result map[string]any, show []string) {
	// Create a set of requested elements
	showSet := make(map[string]bool)
	for _, elem := range show {
		showSet[elem] = true
	}

	// Map tai show elements to Claude's boolean flags
	if len(show) > 0 {
		// Only set flags if show is explicitly specified
		result["showCost"] = showSet["cost"]
		result["showModel"] = showSet["model"]
		result["showTokens"] = showSet["tokens"]
		result["showLatency"] = showSet["latency"]
		// Note: "context" maps to nothing in current Claude Code; reserved for future use
	}
}
