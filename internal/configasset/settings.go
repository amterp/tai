package configasset

import (
	"fmt"
	"sort"
	"strings"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
)

func init() {
	Register(&settingsHandler{})
}

// settingsHandler installs fields into a target's own settings file, each one
// as the smallest entry that can be owned without displacing anyone else's.
type settingsHandler struct{}

func (h *settingsHandler) Type() string { return config.AssetTypeSettings }

// Cardinality is Collection: several settings assets coexist, each owning the
// fields it sets.
func (h *settingsHandler) Cardinality() Cardinality { return Collection }

func (h *settingsHandler) Plan(req Request) (*Plan, error) {
	cfg, err := config.ParseSettingsConfig(req.Content)
	if err != nil {
		return nil, err
	}

	plan := &Plan{Warnings: unknownNativeTargetWarnings(req.Name, cfg)}

	fields := cfg.Native[req.Adapter.Name()]
	if len(fields) == 0 {
		// Settings are target-specific by nature, so an asset written for one
		// target saying nothing to another is the normal case, not a problem.
		return plan, nil
	}

	if err := rejectOwnedKeys(req.Adapter, fields); err != nil {
		return nil, fmt.Errorf("settings %s: %w", req.Name, err)
	}

	file := req.Adapter.ConfigFilePath(config.AssetTypeSettings, req.Scope)
	if file == "" {
		return nil, UnsupportedScopeError(req.Adapter, config.AssetTypeSettings, req.Scope, "settings")
	}

	plan.Edits = FieldEdits(file, nil, fields, req.Name)

	if consenter, ok := req.Adapter.(ConfigWriteConsenter); ok {
		plan.Consent = consenter.ConfigWriteConsent(config.AssetTypeSettings, file)
	}

	return plan, nil
}

// FieldEdits translates a tree of settings fields into edits rooted at base,
// owning each value at the finest grain that leaves everyone else's alone:
//
//   - an object is never owned whole; each of its keys is, recursively, so two
//     assets (or an asset and the user) can set sibling keys of one object
//   - a scalar or empty object is owned as one entry: its key outright at the
//     top level, or as a keyed entry inside its parent object below it
//   - each array element is owned separately and appended, so arrays from
//     several owners accumulate rather than replace one another
//
// owner names whoever writes the fields. Array elements carry it in their ID,
// which is what keeps two owners' elements in one array from being taken for
// the same entry.
//
// Any asset writing fields into a target's settings can build its edits here,
// passing base to root them below a key of its own.
func FieldEdits(file string, base []string, fields map[string]any, owner string) []Edit {
	var edits []Edit
	var walk func(path []string, value any)
	walk = func(path []string, value any) {
		value = jsonValue(value)
		switch v := value.(type) {
		case map[string]any:
			if len(v) > 0 {
				for _, key := range sortedMapKeys(v) {
					walk(appendPath(path, key), v[key])
				}
				return
			}
		case []any:
			for i, elem := range v {
				edits = append(edits, Edit{
					File:    file,
					KeyPath: path,
					Mode:    AppendToArray,
					ID:      fmt.Sprintf("%s[%d]", owner, i),
					Value:   elem,
				})
			}
			return
		}

		if len(path) == 1 {
			edits = append(edits, Edit{File: file, KeyPath: path, Mode: ReplaceKey, Value: value})
			return
		}
		edits = append(edits, Edit{
			File:    file,
			KeyPath: path[:len(path)-1],
			Mode:    UpsertMapEntry,
			ID:      path[len(path)-1],
			Value:   value,
		})
	}

	for _, key := range sortedMapKeys(fields) {
		walk(appendPath(base, key), fields[key])
	}
	return edits
}

// NamesTarget reports whether a settings asset has a block for target. An
// asset that cannot be parsed reports true, so the caller treats it as meant
// for the target and the problem surfaces rather than being skipped quietly.
func NamesTarget(content []byte, target string) bool {
	cfg, err := config.ParseSettingsConfig(content)
	if err != nil {
		return true
	}
	_, ok := cfg.Native[target]
	return ok
}

// rejectOwnedKeys refuses fields that another config asset type owns. Letting
// a settings asset write `hooks` would give one key two owners with different
// merge rules, and whichever installed second would undo the first.
func rejectOwnedKeys(adap adapter.Adapter, fields map[string]any) error {
	for _, t := range Types() {
		if t == config.AssetTypeSettings {
			continue
		}
		key := adap.SettingsKey(t)
		if key == "" {
			continue
		}
		if _, ok := fields[key]; ok {
			return fmt.Errorf("%q is managed by %s assets; write it as one of those instead", key, t)
		}
	}

	// Approval policies own arrays rather than a top-level key, so a settings
	// asset may still set the rest of the object holding them.
	if translator, ok := adap.(ApprovalTranslator); ok {
		for _, kind := range config.ValidApprovalKinds() {
			path := translator.ApprovalKeyPath(kind)
			if path != nil && hasPath(fields, path) {
				return fmt.Errorf("%q is managed by %s assets (%s entries); write it as one of those instead",
					strings.Join(path, "."), config.AssetTypeApproval, kind)
			}
		}
	}
	return nil
}

// hasPath reports whether fields holds a value at path.
func hasPath(fields map[string]any, path []string) bool {
	var current any = fields
	for _, key := range path {
		obj, ok := jsonValue(current).(map[string]any)
		if !ok {
			return false
		}
		if current, ok = obj[key]; !ok {
			return false
		}
	}
	return true
}

// unknownNativeTargetWarnings flags `native` keys naming no target, since
// those fields reach nothing and look exactly like fields that installed.
func unknownNativeTargetWarnings(name string, cfg *config.SettingsConfig) []string {
	known := adapter.TargetNames()
	var warnings []string
	for _, target := range sortedMapKeys(cfg.Native) {
		if containsString(known, target) {
			continue
		}
		if _, removed := adapter.RemovedTarget(target); removed {
			warnings = append(warnings, fmt.Sprintf(
				"settings %q: 'native.%s' names a target tai no longer supports, so those fields reach nothing. Delete them.",
				name, target))
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"settings %q: 'native.%s' names a target tai has no adapter for, so those fields reach nothing. "+
				"Known targets are %s - check the spelling.",
			name, target, strings.Join(known, ", ")))
	}
	return warnings
}

// jsonValue converts a decoded YAML value into one JSON can encode. YAML
// allows non-string map keys and decodes such a map as map[any]any, which
// encoding/json rejects.
func jsonValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = jsonValue(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = jsonValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = jsonValue(val)
		}
		return out
	default:
		return v
	}
}

func appendPath(path []string, key string) []string {
	return append(append([]string(nil), path...), key)
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
