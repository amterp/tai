package configasset

import (
	"fmt"
	"slices"
	"strings"

	"github.com/amterp/tai/internal/config"
)

func init() {
	Register(&statusLineHandler{})
}

// StatusLineTranslator is implemented by adapters that support status lines.
//
// It is an optional interface rather than a method on adapter.Adapter so that
// the base interface does not grow a method per config asset type. An adapter
// declares support in SupportedConfigAssetTypes and implements the matching
// translator; targets without status lines implement neither.
type StatusLineTranslator interface {
	// TransformStatusLine renders a tai status line config into the target's
	// own shape. Exactly one of cfg and rawNative is non-nil: cfg for tai
	// format, rawNative for a native config passed through verbatim.
	TransformStatusLine(cfg *config.StatusLineConfig, rawNative []byte) (map[string]any, error)
}

// statusLineHandler installs a status line as a single owned settings key.
type statusLineHandler struct{}

func (h *statusLineHandler) Type() string { return config.AssetTypeStatusLine }

// Cardinality is Singleton: a target displays one status line, so a second
// status line asset in a profile overrides the first rather than joining it.
func (h *statusLineHandler) Cardinality() Cardinality { return Singleton }

func (h *statusLineHandler) Plan(req Request) (*Plan, error) {
	cfg, rawNative, err := config.ParseStatusLineConfig(req.Content)
	if err != nil {
		return nil, err
	}

	if cfg != nil {
		if err := ValidateStatusLineConfig(cfg); err != nil {
			return nil, fmt.Errorf("validating status line config: %w", err)
		}
	}

	translator, ok := req.Adapter.(StatusLineTranslator)
	if !ok {
		return nil, fmt.Errorf("target %q does not support status lines", req.Adapter.Name())
	}

	value, err := translator.TransformStatusLine(cfg, rawNative)
	if err != nil {
		return nil, fmt.Errorf("transforming status line: %w", err)
	}

	file := req.Adapter.ConfigFilePath(config.AssetTypeStatusLine, req.Scope)
	if file == "" {
		return nil, fmt.Errorf("target %q has no status line config file for scope %q",
			req.Adapter.Name(), req.Scope)
	}

	key := req.Adapter.SettingsKey(config.AssetTypeStatusLine)
	if key == "" {
		return nil, fmt.Errorf("target %q has no settings key for status lines", req.Adapter.Name())
	}

	plan := &Plan{
		Edits: []Edit{{
			File:    file,
			KeyPath: []string{key},
			Mode:    ReplaceKey,
			Value:   value,
		}},
	}

	if cfg == nil {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("status line %s uses native format - passed through to %s untranslated",
				req.Asset.Name, req.Adapter.Name()))
	}

	return plan, nil
}

// statusLineLevels lists every valid status line level.
func statusLineLevels() []string {
	return []string{
		config.StatusLineLevelMinimal,
		config.StatusLineLevelStandard,
		config.StatusLineLevelDetailed,
		config.StatusLineLevelHidden,
		config.StatusLineLevelCustom,
	}
}

// ValidateStatusLineConfig validates a tai-format status line configuration.
func ValidateStatusLineConfig(cfg *config.StatusLineConfig) error {
	if cfg == nil {
		return nil
	}

	if !slices.Contains(statusLineLevels(), cfg.Level) {
		return fmt.Errorf("invalid level %q; must be one of: %s",
			cfg.Level, strings.Join(statusLineLevels(), ", "))
	}

	switch cfg.Level {
	case config.StatusLineLevelCustom:
		if cfg.Command == "" {
			return fmt.Errorf("'command' is required when level is 'custom'")
		}
		if len(cfg.Show) > 0 {
			return fmt.Errorf("'show' is not allowed when level is 'custom'")
		}
	case config.StatusLineLevelHidden:
		if len(cfg.Show) > 0 {
			return fmt.Errorf("'show' is not allowed when level is 'hidden'")
		}
		if cfg.Command != "" {
			return fmt.Errorf("'command' is not allowed when level is 'hidden'")
		}
	default: // minimal, standard, detailed
		if cfg.Command != "" {
			return fmt.Errorf("'command' is only allowed when level is 'custom'")
		}
	}

	// Unknown show elements are silently ignored for forward compatibility, so
	// a config using a newer element still installs on an older tai.

	return nil
}
