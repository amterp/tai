// Package pathutil provides utilities for path resolution and validation.
package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands ~ at the start of a path to the user's home directory.
// Returns the original path if it doesn't start with ~/ or if home cannot be determined.
func ExpandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, path[2:]), nil
}

// MakeAbsolute converts a path to absolute, resolving it relative to base if needed.
// Also expands ~ prefixes.
func MakeAbsolute(path, base string) (string, error) {
	// First expand home directory
	expanded, err := ExpandHome(path)
	if err != nil {
		return "", err
	}

	// If already absolute, just clean it
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}

	// Resolve relative to base
	return filepath.Clean(filepath.Join(base, expanded)), nil
}

// IsContainedIn checks if path is contained within (or equal to) the base directory.
// Both paths are cleaned before comparison. Returns false for path traversal attempts.
func IsContainedIn(path, base string) bool {
	cleanPath := filepath.Clean(path)
	cleanBase := filepath.Clean(base)

	// Exact match
	if cleanPath == cleanBase {
		return true
	}

	// Must be a subdirectory
	return strings.HasPrefix(cleanPath, cleanBase+string(filepath.Separator))
}

// ValidateExists checks if a path exists and returns an error if it doesn't.
func ValidateExists(path string) error {
	_, err := os.Stat(path)
	return err
}

// ValidateIsDir checks if a path is a directory.
func ValidateIsDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &NotADirectoryError{Path: path}
	}
	return nil
}

// NotADirectoryError is returned when a path is expected to be a directory but isn't.
type NotADirectoryError struct {
	Path string
}

func (e *NotADirectoryError) Error() string {
	return "not a directory: " + e.Path
}

// ValidateRelativePath checks that a path is safe for use as a relative destination.
// Returns an error if the path:
// - Is empty
// - Is absolute
// - Contains path traversal sequences (..)
func ValidateRelativePath(path string) error {
	if path == "" {
		return &InvalidPathError{Path: path, Reason: "path is empty"}
	}

	if filepath.IsAbs(path) {
		return &InvalidPathError{Path: path, Reason: "absolute paths not allowed"}
	}

	// Check for path traversal - clean the path and see if it escapes
	cleaned := filepath.Clean(path)
	if strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return &InvalidPathError{Path: path, Reason: "path traversal not allowed"}
	}

	return nil
}

// InvalidPathError is returned when a path fails validation.
type InvalidPathError struct {
	Path   string
	Reason string
}

func (e *InvalidPathError) Error() string {
	return "invalid path '" + e.Path + "': " + e.Reason
}
