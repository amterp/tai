// Package gemini provides the adapter for Gemini CLI.
package gemini

import (
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

func init() {
	adapter.DefaultRegistry.Register(New(""))
}

// Adapter implements the adapter.Adapter interface for Gemini CLI.
type Adapter struct {
	targetDir string // Override for testing
}

// HooksSettingsKey is the top-level key Gemini reads hooks from. Note that
// most Gemini settings are deeply namespaced (ui.theme, model.name), but hooks
// sit at the top level.
const HooksSettingsKey = "hooks"

// hookEvents maps canonical tai events to Gemini's names.
//
// Gemini structures hooks exactly like Claude and Codex but renames every
// event, which is the clearest argument for tai naming events itself rather
// than borrowing one target's vocabulary.
//
// Gemini has no subagent or post-compact events, and no separate permission
// event; hooks using those are skipped with a warning. It also has events with
// no equivalent anywhere else - BeforeModel, AfterModel, BeforeToolSelection,
// which sit inside the model call - that tai does not abstract.
var hookEvents = map[string]string{
	config.HookEventSessionStart:     "SessionStart",
	config.HookEventSessionEnd:       "SessionEnd",
	config.HookEventUserPromptSubmit: "BeforeAgent",
	config.HookEventPreToolUse:       "BeforeTool",
	config.HookEventPostToolUse:      "AfterTool",
	config.HookEventStop:             "AfterAgent",
	config.HookEventPreCompact:       "PreCompress",
	config.HookEventNotification:     "Notification",
}

// hookTools maps canonical tai tool names to Gemini's snake_case tools.
var hookTools = map[string]string{
	config.HookToolRead:  "read_file",
	config.HookToolWrite: "write_file",
	config.HookToolEdit:  "replace",
	config.HookToolShell: "run_shell_command",
}

// New creates a new Gemini adapter.
// If targetDir is empty, defaults to $GEMINI_HOME or ~/.gemini.
func New(targetDir string) *Adapter {
	if targetDir == "" {
		targetDir = os.Getenv("GEMINI_HOME")
	}
	if targetDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			targetDir = ".gemini" // Fallback
		} else {
			targetDir = filepath.Join(home, ".gemini")
		}
	}
	return &Adapter{targetDir: targetDir}
}

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "gemini"
}

// AssetMapping returns how tai asset types map to Gemini paths.
// Gemini CLI has no native persona concept, so personas are mapped to skills.
// Delegates are unsupported and handled via UnsupportedFileAssetTypes.
func (a *Adapter) AssetMapping() map[string]string {
	return map[string]string{
		config.AssetTypePersona: "skills",
		config.AssetTypeSkill:   "skills",
		// Note: delegates are in UnsupportedFileAssetTypes, not mapped here
		// Note: "file" type uses asset.Dest directly, not this mapping
	}
}

// TargetDir returns the Gemini config directory.
func (a *Adapter) TargetDir() string {
	return a.targetDir
}

// MapAssetPath maps a tai asset to a Gemini target path.
// Single-file assets that map to skills/ are wrapped in a directory
// (skills/name/SKILL.md) because Gemini CLI only discovers directory-format skills.
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

// TransformContent transforms asset content into Gemini's expected format.
// Personas are mapped to skills, so they get the same frontmatter (name + description).
// Skills get name + description frontmatter.
// Files are returned as-is.
func (a *Adapter) TransformContent(asset *config.ResolvedAsset) ([]byte, error) {
	switch asset.Type {
	case config.AssetTypePersona, config.AssetTypeSkill:
		return a.transformSkill(asset), nil
	case config.AssetTypeFile:
		return asset.Content, nil
	default:
		return asset.Content, nil
	}
}

// transformSkill renders Gemini skill frontmatter (name + description).
// Used for both skills and personas (which are mapped to skills).
func (a *Adapter) transformSkill(asset *config.ResolvedAsset) []byte {
	fields := map[string]string{fm.FieldName: asset.Metadata[fm.FieldName]}
	order := []string{fm.FieldName}

	if desc := asset.Metadata[fm.FieldDescription]; desc != "" {
		fields[fm.FieldDescription] = desc
		order = append(order, fm.FieldDescription)
	}

	return fm.Render(fields, order, asset.Content)
}

// Run invokes gemini with the given arguments.
func (a *Adapter) Run(args []string) error {
	return adapter.RunBinary("gemini", args)
}

// SupportedConfigAssetTypes returns the config asset types Gemini supports.
// Status lines have no Gemini equivalent, so they are skipped with a warning.
func (a *Adapter) SupportedConfigAssetTypes() []string {
	return []string{config.AssetTypeHook}
}

// UnsupportedFileAssetTypes returns file asset types Gemini doesn't support.
// Delegates are skipped because Gemini CLI has no user-defined sub-agent concept.
func (a *Adapter) UnsupportedFileAssetTypes() []string {
	return []string{config.AssetTypeDelegate}
}

// InstallWarnings returns Gemini-specific advisory install warnings.
func (a *Adapter) InstallWarnings(asset *config.Asset) []string {
	switch asset.Type {
	case config.AssetTypePersona:
		return []string{
			fmt.Sprintf("Gemini CLI does not support personas - installed as skill instead: skills/%s/SKILL.md",
				strings.TrimSuffix(asset.Name, ".md")),
		}
	default:
		return nil
	}
}

// ConfigFilePath returns the Gemini config file for an asset type and scope.
func (a *Adapter) ConfigFilePath(assetType, scope string) string {
	if assetType != config.AssetTypeHook {
		return ""
	}

	switch scope {
	case config.ScopeProject:
		rel := filepath.Join(".gemini", "settings.json")
		abs, err := filepath.Abs(rel)
		if err != nil {
			return rel
		}
		return abs
	case config.ScopeUser:
		return filepath.Join(a.targetDir, "settings.json")
	default:
		// Gemini settings are user, workspace, or system - there is no local
		// scope to map onto.
		return ""
	}
}

// HookEventName maps a canonical tai event to Gemini's name for it.
func (a *Adapter) HookEventName(canonical string) (string, bool) {
	name, ok := hookEvents[canonical]
	return name, ok
}

// HookToolName maps a canonical tai tool name to Gemini's name for it.
func (a *Adapter) HookToolName(canonical string) (string, bool) {
	name, ok := hookTools[canonical]
	return name, ok
}

// HookTimeoutUnit reports that Gemini expects hook timeouts in milliseconds,
// unlike Claude and Codex which take seconds.
func (a *Adapter) HookTimeoutUnit() configasset.TimeoutUnit {
	return configasset.TimeoutMilliseconds
}

// SettingsKey returns the JSON key name for a config asset type.
func (a *Adapter) SettingsKey(assetType string) string {
	if assetType == config.AssetTypeHook {
		return HooksSettingsKey
	}
	return ""
}

// MainFileName returns the main file name for a directory asset type.
func (a *Adapter) MainFileName(assetType string) string {
	return config.MainFileName(assetType)
}

// ProvideVariables returns Gemini-specific template variables.
func (a *Adapter) ProvideVariables() template.Variables {
	return template.Variables{
		"target_name":         "gemini",
		"target_name_display": "Gemini",
		"target_agents_file":  "GEMINI.md",
	}
}
