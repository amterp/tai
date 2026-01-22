// Package output provides an abstraction for CLI output with support for
// both styled (colored) and plain text output.
package output

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Output defines the interface for CLI output operations.
type Output interface {
	// Header displays a section header.
	Header(text string)

	// Blank outputs a blank line (for spacing).
	Blank()

	// List displays a titled list of items.
	List(title string, items []ListItem)

	// KeyValue displays a single key-value pair.
	KeyValue(key, value string)

	// KeyValueBlock displays multiple key-value pairs.
	KeyValueBlock(pairs []KeyValuePair)

	// InstallResult displays installation results with color-coded +/~/- prefixes.
	InstallResult(data *InstallResultData)

	// Success displays a success message.
	Success(msg string)

	// Info displays an informational message.
	Info(msg string)

	// Warning displays a warning message.
	Warning(msg string)

	// Error displays an error message to stderr.
	Error(msg string)

	// Raw outputs text without any formatting (for scripting).
	Raw(text string)
}

// ListItem represents an item in a list display.
type ListItem struct {
	Name        string
	Description string
	Extra       string // Additional info (e.g., path, source)
}

// KeyValuePair represents a key-value pair for display.
type KeyValuePair struct {
	Key   string
	Value string
}

// InstallResultData contains the data for displaying installation results.
type InstallResultData struct {
	Added    []string
	Updated  []string
	Removed  []string
	Skipped  []string
	DryRun   bool
	Manifest string // Path to manifest file (empty in dry-run)
}

// Options configures output behavior.
type Options struct {
	Writer    io.Writer
	ErrWriter io.Writer
	NoColor   bool
}

// New creates an Output instance based on the provided options.
// It automatically detects TTY and falls back to plain output when appropriate.
func New(opts Options) Output {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.ErrWriter == nil {
		opts.ErrWriter = os.Stderr
	}

	if opts.NoColor || !isTerminal(opts.Writer) {
		return NewPlain(opts.Writer, opts.ErrWriter)
	}
	return NewStyled(opts.Writer, opts.ErrWriter)
}

// isTerminal checks if the writer is a terminal.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}
