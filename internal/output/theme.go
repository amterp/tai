package output

import "github.com/charmbracelet/lipgloss"

// ANSI color codes for terminal output.
// Using the standard 16-color palette for broad terminal compatibility.
const (
	colorGray   = lipgloss.Color("8")
	colorRed    = lipgloss.Color("9")
	colorGreen  = lipgloss.Color("10")
	colorYellow = lipgloss.Color("11")
	colorBlue   = lipgloss.Color("12")
)

// Theme defines the color and style definitions for styled output.
type Theme struct {
	// Headers (section titles)
	Header lipgloss.Style

	// Status indicators
	Added   lipgloss.Style // + prefix for new items
	Updated lipgloss.Style // ~ prefix for updated items
	Removed lipgloss.Style // - prefix for removed items
	Skipped lipgloss.Style // ? prefix for skipped items

	// Messages
	Success lipgloss.Style
	Info    lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style

	// Key-value display
	Key lipgloss.Style
}

// DefaultTheme returns the default color theme.
func DefaultTheme() Theme {
	return Theme{
		Header: lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true),

		Added:   lipgloss.NewStyle().Foreground(colorGreen),
		Updated: lipgloss.NewStyle().Foreground(colorYellow),
		Removed: lipgloss.NewStyle().Foreground(colorRed),
		Skipped: lipgloss.NewStyle().Foreground(colorGray),

		Success: lipgloss.NewStyle().Foreground(colorGreen),
		Info:    lipgloss.NewStyle().Foreground(colorBlue),
		Warning: lipgloss.NewStyle().Foreground(colorYellow),
		Error:   lipgloss.NewStyle().Foreground(colorRed).Bold(true),

		Key: lipgloss.NewStyle().Bold(true),
	}
}
