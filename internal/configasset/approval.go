package configasset

import (
	"fmt"
	"strings"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
)

func init() {
	Register(&approvalHandler{})
}

// ApprovalTranslator is implemented by adapters that support approval
// policies.
//
// The entries are the same prose on every target; what differs is where a
// target keeps them, which is all this asks.
type ApprovalTranslator interface {
	// ApprovalKeyPath locates the array holding entries of kind. nil means
	// this target has no place for that kind, which skips those entries with a
	// warning.
	ApprovalKeyPath(kind string) []string

	// ApprovalArraySeed is the element an array tai creates must start with,
	// or nil when an array holding only tai's entries means what it says.
	ApprovalArraySeed() any
}

// approvalHandler installs approval policy entries as owned elements of the
// target's policy arrays.
type approvalHandler struct{}

func (h *approvalHandler) Type() string { return config.AssetTypeApproval }

// Cardinality is Collection: each repo contributes its own policy file, and
// they coexist.
func (h *approvalHandler) Cardinality() Cardinality { return Collection }

func (h *approvalHandler) Plan(req Request) (*Plan, error) {
	entries, err := config.ParseApprovalPolicy(req.Content)
	if err != nil {
		return nil, fmt.Errorf("approval %s: %w", req.Name, err)
	}
	if err := validateApprovalTargets(entries); err != nil {
		return nil, fmt.Errorf("approval %s: %w", req.Name, err)
	}

	translator, ok := req.Adapter.(ApprovalTranslator)
	if !ok {
		return nil, fmt.Errorf("target %q does not support approval policies", req.Adapter.Name())
	}

	plan := &Plan{}
	texts := map[string][]any{}
	var kinds []string
	for _, entry := range entries {
		if !entry.AppliesTo(req.Adapter.Name()) {
			continue
		}
		if translator.ApprovalKeyPath(entry.Kind) == nil {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"approval %s: target %s has no place for %q entries - skipping %q",
				req.Name, req.Adapter.Name(), entry.Kind, entry.Title))
			continue
		}
		if _, seen := texts[entry.Kind]; !seen {
			kinds = append(kinds, entry.Kind)
		}
		texts[entry.Kind] = append(texts[entry.Kind], approvalText(entry))
	}
	if len(kinds) == 0 {
		return plan, nil
	}

	file := req.Adapter.ConfigFilePath(config.AssetTypeApproval, req.Scope)
	if file == "" {
		return nil, approvalScopeError(req.Adapter, req.Scope)
	}

	for _, kind := range kinds {
		path := translator.ApprovalKeyPath(kind)
		if seed := translator.ApprovalArraySeed(); seed != nil {
			plan.Edits = append(plan.Edits, Edit{File: file, KeyPath: path, Mode: SeedArray, Value: seed})
		}
		parent, key := path[:len(path)-1], path[len(path)-1]
		plan.Edits = append(plan.Edits, FieldEdits(file, parent, map[string]any{key: texts[kind]}, req.Name)...)
	}

	if consenter, ok := req.Adapter.(ConfigWriteConsenter); ok {
		plan.Consent = consenter.ConfigWriteConsent(config.AssetTypeApproval, file)
	}

	return plan, nil
}

// approvalText renders an entry as the one string a reviewer reads.
func approvalText(entry config.ApprovalEntry) string {
	if entry.Body == "" {
		return entry.Title
	}
	return entry.Title + ": " + entry.Body
}

// approvalScopeError explains a scope the target reads no policy from.
// Writing it there anyway would install something the reviewer never sees.
func approvalScopeError(adap adapter.Adapter, scope string) error {
	var supported []string
	for _, s := range config.ValidScopes() {
		if adap.ConfigFilePath(config.AssetTypeApproval, s) != "" {
			supported = append(supported, s)
		}
	}
	if len(supported) == 0 {
		return fmt.Errorf("target %q does not install approval policies at any scope", adap.Name())
	}
	return fmt.Errorf(
		"target %q reads its approval policy only at scope %s, not %q, so an entry installed there would never be seen. "+
			"Set `scope: %s` on this asset's entry in the profile or bundle",
		adap.Name(), strings.Join(supported, ", "), scope, supported[0])
}

func validateApprovalTargets(entries []config.ApprovalEntry) error {
	known := adapter.TargetNames()
	for _, entry := range entries {
		for _, target := range entry.Targets {
			if !containsString(known, target) {
				return fmt.Errorf("entry %q names unknown target %q; known targets are %s",
					entry.Title, target, strings.Join(known, ", "))
			}
		}
	}
	return nil
}

// ApprovalAppliesTo reports whether any entry of an approval asset is meant
// for target. An asset that cannot be parsed reports true, so the problem
// surfaces rather than the asset being skipped quietly.
func ApprovalAppliesTo(content []byte, target string) bool {
	entries, err := config.ParseApprovalPolicy(content)
	if err != nil {
		return true
	}
	for _, entry := range entries {
		if entry.AppliesTo(target) {
			return true
		}
	}
	return false
}
