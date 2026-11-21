// Package antigravity provides the adapter for Antigravity CLI.
package antigravity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/configasset"
	fm "github.com/amterp/tai/internal/frontmatter"
	"github.com/amterp/tai/internal/template"
)

const (
	targetName    = "antigravity"
	statusLineKey = "statusLine"
	mcpServersKey = "mcpServers"
	preToolUse    = "PreToolUse"
	postToolUse   = "PostToolUse"
)

var hookEvents = map[string]string{
	config.HookEventSessionStart:   "SessionStart",
	config.HookEventPreInvocation:  "PreInvocation",
	config.HookEventPostInvocation: "PostInvocation",
	config.HookEventPreToolUse:     preToolUse,
	config.HookEventPostToolUse:    postToolUse,
	config.HookEventStop:           "Stop",
}

// Match both the names reported in 1.5.14 hook payloads and the tool names
// listed in Google's hook guide.
var hookTools = map[string]string{
	config.HookToolRead:  "read_file|view_file",
	config.HookToolWrite: "write_file|write_to_file",
	config.HookToolEdit:  "edit_file|replace_file_content|multi_replace_file_content",
	config.HookToolShell: "run_command",
}

func init() {
	adapter.DefaultRegistry.Register(New(""))
}

type Adapter struct {
	targetDir string
}

// New uses ~/.gemini unless a target directory is supplied.
func New(targetDir string) *Adapter {
	if targetDir == "" {
		home, _ := os.UserHomeDir()
		targetDir = filepath.Join(home, ".gemini")
	}
	return &Adapter{targetDir: targetDir}
}

func (a *Adapter) Name() string      { return targetName }
func (a *Adapter) TargetDir() string { return a.targetDir }

func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypeDelegate: filepath.Join("config", "agents"),
		config.AssetTypeSkill:    filepath.Join("config", "skills"),
	}
}

func (a *Adapter) MapAssetPath(asset *config.Asset) string {
	mapped := adapter.DefaultMapAssetPath(a.AssetMapping(), asset)
	if asset.IsDir {
		return mapped
	}
	switch asset.Type {
	case config.AssetTypeSkill:
		return filepath.Join(withoutExtension(mapped), config.MainFileSkill)
	case config.AssetTypeDelegate:
		return withoutExtension(mapped) + ".md"
	default:
		return mapped
	}
}

func withoutExtension(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(filepath.Base(path)))
}

func (a *Adapter) MainFileName(assetType string) string {
	if assetType == config.AssetTypeDelegate {
		return "agent.md"
	}
	return config.MainFileName(assetType)
}

func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	if asset.Type != config.AssetTypeDelegate && asset.Type != config.AssetTypeSkill {
		return asset.Content, nil
	}

	name := asset.Metadata[fm.FieldName]
	description := asset.Metadata[fm.FieldDescription]
	if description == "" {
		description = name
	}
	var fields fm.Fields
	fields.Set(fm.FieldName, name)
	fields.Set(fm.FieldDescription, description)

	native := asset.Native[a.Name()]
	if _, ok := native[fm.FieldModel]; !ok {
		model := asset.Metadata[fm.FieldModel]
		switch {
		case model == "":
		case asset.Type == config.AssetTypeDelegate && (model == "inherit" || model == "flash" || model == "pro"):
			fields.Set(fm.FieldModel, model)
		default:
			asset.Warnings = append(asset.Warnings,
				fmt.Sprintf("Antigravity cannot use model %q from %s - set native.antigravity.model for a custom agent", model, asset.Name))
		}
	}
	if _, ok := native[fm.FieldTools]; !ok && asset.Metadata[fm.FieldTools] != "" {
		asset.Warnings = append(asset.Warnings,
			fmt.Sprintf("Antigravity dropped tools from %s - use native.antigravity.tools with Antigravity tool names for a custom agent", asset.Name))
	}

	fields.Merge(native)
	return fm.Render(fields, asset.Content), nil
}

func (a *Adapter) Run(args []string) error { return adapter.RunBinary("agy", args) }

func (a *Adapter) SupportedConfigAssetTypes() []string {
	return []string{config.AssetTypeStatusLine, config.AssetTypeHook, config.AssetTypeMCP, config.AssetTypeSettings}
}

func (a *Adapter) UnsupportedFileAssetTypes() []string { return []string{config.AssetTypePersona} }

func (a *Adapter) InstallWarnings(_ *config.Asset) []string { return nil }

func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         a.Name(),
		"target_name_display": "Antigravity",
		"target_agents_file":  "AGENTS.md",
	}
}

func (a *Adapter) ConfigFilePath(assetType, scope string) string {
	var filename string
	switch assetType {
	case config.AssetTypeStatusLine, config.AssetTypeSettings:
		if scope == config.ScopeUser {
			return filepath.Join(a.targetDir, "antigravity-cli", "settings.json")
		}
		return ""
	case config.AssetTypeHook:
		filename = "hooks.json"
	case config.AssetTypeMCP:
		filename = "mcp_config.json"
	default:
		return ""
	}

	switch scope {
	case config.ScopeUser:
		return filepath.Join(a.targetDir, "config", filename)
	case config.ScopeProject:
		path, err := filepath.Abs(filepath.Join(".agents", filename))
		if err == nil {
			return path
		}
	}
	return ""
}

func (a *Adapter) SettingsKey(assetType string) string {
	switch assetType {
	case config.AssetTypeStatusLine:
		return statusLineKey
	case config.AssetTypeMCP:
		return mcpServersKey
	default:
		return ""
	}
}

func (a *Adapter) HookEventName(canonical string) (string, bool) {
	event, ok := hookEvents[canonical]
	return event, ok
}

func (a *Adapter) HookToolName(canonical string) (string, bool) {
	tool, ok := hookTools[canonical]
	return tool, ok
}

func (a *Adapter) HookTimeoutUnit() configasset.TimeoutUnit { return configasset.TimeoutSeconds }

func (a *Adapter) HookEdit(scope string, hook configasset.TranslatedHook) (configasset.Edit, error) {
	file := a.ConfigFilePath(config.AssetTypeHook, scope)
	if file == "" {
		return configasset.Edit{}, configasset.UnsupportedScopeError(a, config.AssetTypeHook, scope, "hooks")
	}

	var entry any = hook.Handler
	switch hook.Event {
	case preToolUse, postToolUse:
		if hook.Matcher == "" {
			hook.Matcher = "*"
		}
		entry = hook.MatcherGroup()
	default:
		if hook.Matcher != "" {
			return configasset.Edit{}, fmt.Errorf("hook %s: Antigravity %s does not accept a tool matcher", hook.Name, hook.Event)
		}
	}
	return configasset.Edit{
		File:    file,
		KeyPath: []string{hook.Name},
		Mode:    configasset.ReplaceKey,
		Value:   map[string]any{hook.Event: []any{entry}},
	}, nil
}

func (a *Adapter) TransformStatusLine(cfg *config.StatusLineConfig, rawNative []byte) (map[string]any, error) {
	if cfg == nil {
		var native map[string]any
		if err := json.Unmarshal(rawNative, &native); err != nil {
			return nil, fmt.Errorf("native Antigravity status line must be a JSON object: %w", err)
		}
		if native == nil {
			return nil, fmt.Errorf("native Antigravity status line must be a JSON object")
		}
		return native, nil
	}
	if cfg.Level != config.StatusLineLevelCustom {
		return nil, fmt.Errorf("status lines for Antigravity require 'level: custom' with a command")
	}
	entry := map[string]any{"type": "command", "command": cfg.Command}
	if cfg.Padding != nil {
		entry["padding"] = *cfg.Padding
	}
	return entry, nil
}

func (a *Adapter) MCPKeyPath(scope string) []string {
	if scope == config.ScopeUser || scope == config.ScopeProject {
		return []string{mcpServersKey}
	}
	return nil
}

func (a *Adapter) TransformMCP(_ string, cfg *config.MCPConfig) (map[string]any, []string, error) {
	entry := map[string]any{}
	switch cfg.Transport {
	case config.MCPTransportStdio:
		entry["command"] = cfg.Command
		if len(cfg.Args) > 0 {
			entry["args"] = configasset.AnySlice(cfg.Args)
		}
		if len(cfg.Env) > 0 {
			entry["env"] = configasset.AnyMap(cfg.Env)
		}
		if cfg.Cwd != "" {
			entry["cwd"] = cfg.Cwd
		}
	case config.MCPTransportHTTP:
		entry["serverUrl"] = cfg.URL
		if len(cfg.Headers) > 0 {
			entry["headers"] = configasset.AnyMap(cfg.Headers)
		}
	default:
		return nil, nil, fmt.Errorf("unsupported Antigravity MCP transport %q", cfg.Transport)
	}

	var warnings []string
	if cfg.Description != "" {
		warnings = append(warnings, "Antigravity MCP entries have no 'description' field - dropped")
	}

	native := cfg.Native[a.Name()]
	for key, value := range entry {
		if _, overridden := native[key]; !overridden && hasEnvRef(value) {
			warnings = append(warnings, "Antigravity MCP environment references are written literally. Expansion in standalone configs is undocumented")
			break
		}
	}
	for key, value := range native {
		entry[key] = value
	}
	return entry, warnings, nil
}

func hasEnvRef(value any) bool {
	switch value := value.(type) {
	case string:
		return configasset.HasEnvRef(value)
	case []any:
		for _, v := range value {
			if hasEnvRef(v) {
				return true
			}
		}
	case map[string]any:
		for _, v := range value {
			if hasEnvRef(v) {
				return true
			}
		}
	}
	return false
}
