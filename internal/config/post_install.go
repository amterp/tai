package config

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PostInstallStep is one command tai runs after an install. Steps run one
// after another, each whether or not earlier ones failed, so one broken
// command cannot hold back the rest.
type PostInstallStep struct {
	// Name identifies the step in reports. Required in the list form; empty
	// only for the single-argv shorthand.
	Name string `yaml:"name"`

	// Command is the argv to execute, run directly with no shell.
	Command []string `yaml:"command"`

	// Timeout bounds this step, parsed via time.ParseDuration. Empty falls
	// back to whatever the declaring config provides.
	Timeout string `yaml:"timeout"`

	// shorthand marks the step built from a plain argv. A name being empty is
	// not enough to tell: a one-step list that forgot its name looks the same,
	// and must be rejected rather than taken for the shorthand.
	shorthand bool
}

var postInstallStepKeys = []string{"name", "command", "timeout"}

// UnmarshalYAML rejects unknown keys. yaml.v3 would otherwise drop them, and a
// misspelled `timeout` would silently run the step under the default.
func (s *PostInstallStep) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a post_install step must be a mapping with 'name' and 'command'", node.Line)
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if !slices.Contains(postInstallStepKeys, key.Value) {
			return fmt.Errorf("line %d: unknown post_install step field %q; valid fields: %s",
				key.Line, key.Value, strings.Join(postInstallStepKeys, ", "))
		}
	}

	type raw PostInstallStep
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}
	*s = PostInstallStep(r)
	return nil
}

// ResolvedTimeout returns the step's own timeout, else fallback, else
// DefaultCommandTimeout.
func (s PostInstallStep) ResolvedTimeout(fallback string) time.Duration {
	if s.Timeout != "" {
		return resolveTimeout(s.Timeout)
	}
	return resolveTimeout(fallback)
}

// PostInstallSteps is a `post_install` list. It accepts a list of steps, or a
// plain argv as shorthand for one unnamed step - the original form, which
// existing configs still use.
type PostInstallSteps []PostInstallStep

const postInstallFormsHelp = "write either an argv list:\n" +
	"  post_install: [./scripts/after-install.sh]\n" +
	"or a list of named steps:\n" +
	"  post_install:\n" +
	"    - name: notify\n" +
	"      command: [./scripts/after-install.sh]"

func (ps *PostInstallSteps) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: 'post_install' must be a list; %s", node.Line, postInstallFormsHelp)
	}
	if len(node.Content) == 0 {
		*ps = nil
		return nil
	}

	scalars := 0
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode {
			scalars++
		}
	}

	switch scalars {
	case len(node.Content):
		var argv []string
		if err := node.Decode(&argv); err != nil {
			return err
		}
		*ps = PostInstallSteps{{Command: argv, shorthand: true}}
		return nil
	case 0:
		var steps []PostInstallStep
		if err := node.Decode(&steps); err != nil {
			return err
		}
		*ps = steps
		return nil
	default:
		return fmt.Errorf("line %d: 'post_install' mixes argv strings with steps; %s", node.Line, postInstallFormsHelp)
	}
}

// IsShorthand reports whether the steps came from the single-argv form.
func (ps PostInstallSteps) IsShorthand() bool {
	return len(ps) == 1 && ps[0].shorthand
}

// validatePostInstallSteps checks every step can run and, in the list form,
// has a name unique among its siblings. Names are how failures are reported,
// so two steps sharing one would make a report ambiguous.
func validatePostInstallSteps(steps PostInstallSteps, field string) error {
	if steps.IsShorthand() {
		return validateCommandArgv(steps[0].Command, field)
	}

	seen := make(map[string]bool, len(steps))
	for idx, step := range steps {
		if step.Name == "" {
			return fmt.Errorf("%s[%d]: missing 'name'", field, idx)
		}
		label := fmt.Sprintf("%s step %q", field, step.Name)
		if seen[step.Name] {
			return fmt.Errorf("%s: duplicate step name; step names must be unique", label)
		}
		seen[step.Name] = true
		if len(step.Command) == 0 {
			return fmt.Errorf("%s: missing 'command'", label)
		}
		if err := validateCommandArgv(step.Command, "command"); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if err := validateTimeoutString(step.Timeout, "timeout"); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
	}
	return nil
}
