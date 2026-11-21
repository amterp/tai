package configasset

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
)

func init() {
	Register(&hookHandler{})
}

// TimeoutUnit is the unit a target expects hook timeouts in. Claude and Codex
// take seconds; others, such as the since-removed Gemini CLI, take
// milliseconds. Getting this wrong fails silently - a 30-second timeout becomes
// 30 milliseconds - so hooks carry a duration and each adapter states its unit
// once, here.
type TimeoutUnit int

const (
	TimeoutSeconds TimeoutUnit = iota
	TimeoutMilliseconds
)

// HookTranslator is implemented by adapters that support hooks.
type HookTranslator interface {
	// HookEventName maps a canonical tai event to this target's name for it.
	// ok is false when the target has no equivalent, which skips the hook with
	// a warning rather than failing the install.
	HookEventName(canonical string) (name string, ok bool)

	// HookToolName maps a canonical tai tool name to this target's name for
	// it. ok is false when the target has no such tool.
	HookToolName(canonical string) (name string, ok bool)

	// HookTimeoutUnit reports the unit this target expects timeouts in.
	HookTimeoutUnit() TimeoutUnit

	// HookEdit places a translated hook in the target's config. It returns
	// an error if the scope or matcher is unsupported.
	HookEdit(scope string, hook TranslatedHook) (Edit, error)
}

// TranslatedHook contains the target's event, matcher, and command before
// the adapter places them in its config file.
type TranslatedHook struct {
	Name    string
	Event   string
	Matcher string
	Handler map[string]any
}

type hookHandler struct{}

func (h *hookHandler) Type() string { return config.AssetTypeHook }

// Cardinality is Collection: a profile carries as many hooks as it likes, and
// several can share one event.
func (h *hookHandler) Cardinality() Cardinality { return Collection }

func (h *hookHandler) Plan(req Request) (*Plan, error) {
	cfg, err := config.ParseHookConfig(req.Content)
	if err != nil {
		return nil, err
	}
	if err := ValidateHookConfig(cfg); err != nil {
		return nil, fmt.Errorf("validating hook %s: %w", req.Asset.Name, err)
	}

	translator, ok := req.Adapter.(HookTranslator)
	if !ok {
		return nil, fmt.Errorf("target %q does not support hooks", req.Adapter.Name())
	}

	plan := &Plan{}

	event, ok := translator.HookEventName(cfg.On)
	if !ok {
		// The type is supported but this particular event is not. Skipping one
		// hook is friendlier than failing the install, since a single profile
		// may install to several targets with different event vocabularies.
		plan.Warnings = append(plan.Warnings, fmt.Sprintf(
			"hook %s: target %s has no %q event - skipping this hook",
			req.Name, req.Adapter.Name(), cfg.On))
		return plan, nil
	}

	matcher, err := hookMatcher(cfg, translator, req.Adapter.Name())
	if err != nil {
		return nil, fmt.Errorf("hook %s: %w", req.Name, err)
	}

	edit, err := translator.HookEdit(req.Scope, TranslatedHook{
		Name:    req.Name,
		Event:   event,
		Matcher: matcher,
		Handler: hookHandlerObject(cfg, translator, req.Adapter.Name()),
	})
	if err != nil {
		return nil, err
	}

	plan.Edits = append(plan.Edits, edit)
	return plan, nil
}

// GroupedHookEdit appends a matcher group to an event under a settings key.
func GroupedHookEdit(adap adapter.Adapter, scope string, hook TranslatedHook) (Edit, error) {
	file := adap.ConfigFilePath(config.AssetTypeHook, scope)
	if file == "" {
		return Edit{}, UnsupportedScopeError(adap, config.AssetTypeHook, scope, "hooks")
	}

	key := adap.SettingsKey(config.AssetTypeHook)
	if key == "" {
		return Edit{}, fmt.Errorf("target %q has no settings key for hooks", adap.Name())
	}

	return Edit{
		File:    file,
		KeyPath: []string{key, hook.Event},
		Mode:    AppendToArray,
		ID:      hook.Name,
		Value:   hook.MatcherGroup(),
	}, nil
}

func (hook TranslatedHook) MatcherGroup() map[string]any {
	group := map[string]any{"hooks": []any{hook.Handler}}
	if hook.Matcher != "" {
		group["matcher"] = hook.Matcher
	}
	return group
}

// hookMatcher builds the target's matcher string.
//
// A per-target `match` wins outright: it exists precisely for patterns the
// canonical vocabulary cannot express. Otherwise canonical tool names are
// translated and joined. An empty result matches everything.
func hookMatcher(cfg *config.HookConfig, t HookTranslator, target string) (string, error) {
	if raw, ok := cfg.Match[target]; ok {
		return raw, nil
	}
	if len(cfg.Tools) == 0 {
		return "", nil
	}

	names := make([]string, 0, len(cfg.Tools))
	for _, tool := range cfg.Tools {
		name, ok := t.HookToolName(tool)
		if !ok {
			// Silently dropping a tool would widen the matcher to every tool -
			// a hook meant to guard writes would start firing on reads. Fail
			// instead, and point at the escape hatch.
			return "", fmt.Errorf(
				"target %s has no tool matching %q; use `match: {%s: ...}` to give it a target-specific matcher",
				target, tool, target)
		}
		names = append(names, name)
	}

	return strings.Join(names, "|"), nil
}

// hookHandlerObject builds the command handler, applying the target's timeout
// unit and merging any native overrides over the top.
func hookHandlerObject(cfg *config.HookConfig, t HookTranslator, target string) map[string]any {
	handler := map[string]any{
		"type":    "command",
		"command": cfg.Run,
	}

	if d, ok := cfg.ResolvedTimeout(); ok {
		switch t.HookTimeoutUnit() {
		case TimeoutMilliseconds:
			handler["timeout"] = d.Milliseconds()
		default:
			// Round up: a sub-second timeout truncating to 0 would read as
			// "no timeout" or "expire immediately" depending on the target.
			seconds := int64(d.Seconds())
			if seconds == 0 && d > 0 {
				seconds = 1
			}
			handler["timeout"] = seconds
		}
	}

	for key, value := range cfg.Native[target] {
		handler[key] = value
	}

	return handler
}

// ValidateHookConfig checks a hook definition.
func ValidateHookConfig(cfg *config.HookConfig) error {
	if cfg == nil {
		return fmt.Errorf("empty hook config")
	}
	if cfg.On == "" {
		return fmt.Errorf("'on' is required (the event that triggers the hook)")
	}
	if !config.IsValidHookEvent(cfg.On) {
		return fmt.Errorf("unknown event %q; must be one of: %s",
			cfg.On, strings.Join(sortedCopy(config.ValidHookEvents()), ", "))
	}
	if strings.TrimSpace(cfg.Run) == "" {
		return fmt.Errorf("'run' is required (the command to execute)")
	}

	for _, tool := range cfg.Tools {
		if !containsString(config.ValidHookTools(), tool) {
			return fmt.Errorf("unknown tool %q; must be one of: %s (use `match` for target-specific patterns)",
				tool, strings.Join(config.ValidHookTools(), ", "))
		}
	}

	if cfg.Timeout != "" {
		if _, ok := cfg.ResolvedTimeout(); !ok {
			return fmt.Errorf("invalid timeout %q; expected a duration such as \"30s\" or \"2m\"", cfg.Timeout)
		}
	}

	return nil
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
