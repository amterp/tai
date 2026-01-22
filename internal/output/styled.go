package output

import (
	"fmt"
	"io"
)

// StyledOutput provides styled output using lipgloss.
type StyledOutput struct {
	out   io.Writer
	err   io.Writer
	theme Theme
}

// NewStyled creates a new StyledOutput instance with the default theme.
func NewStyled(out, err io.Writer) *StyledOutput {
	return &StyledOutput{
		out:   out,
		err:   err,
		theme: DefaultTheme(),
	}
}

func (s *StyledOutput) Header(text string) {
	fmt.Fprintln(s.out, s.theme.Header.Render(text))
}

func (s *StyledOutput) Blank() {
	fmt.Fprintln(s.out)
}

func (s *StyledOutput) List(title string, items []ListItem) {
	fmt.Fprintln(s.out, s.theme.Header.Render(title))
	for _, item := range items {
		if item.Extra != "" {
			fmt.Fprintf(s.out, "  %-15s  %-10s  %s\n", item.Name, item.Description, item.Extra)
		} else if item.Description != "" {
			fmt.Fprintf(s.out, "  %-20s  %s\n", item.Name, item.Description)
		} else {
			fmt.Fprintf(s.out, "  %s\n", item.Name)
		}
	}
}

func (s *StyledOutput) KeyValue(key, value string) {
	fmt.Fprintf(s.out, "%s: %s\n", s.theme.Key.Render(key), value)
}

func (s *StyledOutput) KeyValueBlock(pairs []KeyValuePair) {
	// Calculate max key width for alignment (including colon)
	maxWidth := 0
	for _, pair := range pairs {
		keyLen := len(pair.Key) + 1 // +1 for colon
		if keyLen > maxWidth {
			maxWidth = keyLen
		}
	}

	for _, pair := range pairs {
		// Render key with style, then pad for alignment
		styledKey := s.theme.Key.Render(pair.Key + ":")
		// Add padding after the styled key
		padding := maxWidth - len(pair.Key) - 1
		fmt.Fprintf(s.out, "%s%*s  %s\n", styledKey, padding, "", pair.Value)
	}
}

func (s *StyledOutput) InstallResult(data *InstallResultData) {
	for _, path := range data.Added {
		fmt.Fprintf(s.out, "  %s %s (new)\n", s.theme.Added.Render("+"), path)
	}
	for _, path := range data.Updated {
		fmt.Fprintf(s.out, "  %s %s (updated)\n", s.theme.Updated.Render("~"), path)
	}
	for _, path := range data.Removed {
		fmt.Fprintf(s.out, "  %s %s (removed)\n", s.theme.Removed.Render("-"), path)
	}
	for _, path := range data.Skipped {
		fmt.Fprintf(s.out, "  %s %s (skipped)\n", s.theme.Skipped.Render("?"), path)
	}

	fmt.Fprintln(s.out)
	if data.DryRun {
		fmt.Fprintf(s.out, "%d would be added, %d updated, %d removed\n",
			len(data.Added), len(data.Updated), len(data.Removed))
	} else {
		fmt.Fprintf(s.out, "%d added, %d updated, %d removed\n",
			len(data.Added), len(data.Updated), len(data.Removed))
		if data.Manifest != "" {
			fmt.Fprintf(s.out, "Manifest written to %s\n", data.Manifest)
		}
	}
}

func (s *StyledOutput) Success(msg string) {
	fmt.Fprintln(s.out, s.theme.Success.Render(msg))
}

func (s *StyledOutput) Info(msg string) {
	fmt.Fprintf(s.out, "%s %s\n", s.theme.Info.Render("ℹ"), msg)
}

func (s *StyledOutput) Warning(msg string) {
	fmt.Fprintf(s.out, "%s %s\n", s.theme.Warning.Render("Warning:"), msg)
}

func (s *StyledOutput) Error(msg string) {
	fmt.Fprintf(s.err, "%s %s\n", s.theme.Error.Render("Error:"), msg)
}

func (s *StyledOutput) Raw(text string) {
	fmt.Fprintln(s.out, text)
}
