package config

import (
	"fmt"
	"time"
)

// resolveTimeout parses a user-specified duration string, falling back to
// DefaultCommandTimeout when the string is empty or (if validation was
// skipped) malformed.
func resolveTimeout(timeout string) time.Duration {
	if timeout == "" {
		return DefaultCommandTimeout
	}
	d, err := time.ParseDuration(timeout)
	if err != nil || d <= 0 {
		return DefaultCommandTimeout
	}
	return d
}

// validateCommandArgv checks a user-declared argv list has no empty elements.
// field names the YAML key for error messages.
func validateCommandArgv(argv []string, field string) error {
	for idx, arg := range argv {
		if arg == "" {
			return fmt.Errorf("'%s' has empty string at index %d", field, idx)
		}
	}
	return nil
}

// validateTimeoutString checks a user-specified duration string parses to a
// positive duration. Empty is allowed (the default applies).
func validateTimeoutString(timeout, field string) error {
	if timeout == "" {
		return nil
	}
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("'%s' %q is not a valid duration: %w", field, timeout, err)
	}
	if d <= 0 {
		return fmt.Errorf("'%s' must be positive, got %q", field, timeout)
	}
	return nil
}
