package output

import (
	"fmt"
	"io"
)

// PlainOutput provides plain text output without any styling.
// It's used for non-TTY environments, tests, and when --no-color is set.
type PlainOutput struct {
	out io.Writer
	err io.Writer
}

// NewPlain creates a new PlainOutput instance.
func NewPlain(out, err io.Writer) *PlainOutput {
	return &PlainOutput{out: out, err: err}
}

func (p *PlainOutput) Header(text string) {
	fmt.Fprintln(p.out, text)
}

func (p *PlainOutput) Blank() {
	fmt.Fprintln(p.out)
}

func (p *PlainOutput) List(title string, items []ListItem) {
	fmt.Fprintln(p.out, title)
	for _, item := range items {
		if item.Extra != "" {
			fmt.Fprintf(p.out, "  %-15s  %-10s  %s\n", item.Name, item.Description, item.Extra)
		} else if item.Description != "" {
			fmt.Fprintf(p.out, "  %-20s  %s\n", item.Name, item.Description)
		} else {
			fmt.Fprintf(p.out, "  %s\n", item.Name)
		}
	}
}

func (p *PlainOutput) KeyValue(key, value string) {
	fmt.Fprintf(p.out, "%s: %s\n", key, value)
}

func (p *PlainOutput) KeyValueBlock(pairs []KeyValuePair) {
	// Calculate max key width for alignment (including colon)
	maxWidth := 0
	for _, pair := range pairs {
		keyLen := len(pair.Key) + 1 // +1 for colon
		if keyLen > maxWidth {
			maxWidth = keyLen
		}
	}

	for _, pair := range pairs {
		fmt.Fprintf(p.out, "%-*s  %s\n", maxWidth, pair.Key+":", pair.Value)
	}
}

func (p *PlainOutput) InstallResult(data *InstallResultData) {
	for _, path := range data.Added {
		fmt.Fprintf(p.out, "  + %s (new)\n", path)
	}
	for _, path := range data.Restored {
		fmt.Fprintf(p.out, "  + %s (restored)\n", path)
	}
	for _, path := range data.Updated {
		fmt.Fprintf(p.out, "  ~ %s (updated)\n", path)
	}
	for _, path := range data.Unchanged {
		fmt.Fprintf(p.out, "  = %s (unchanged)\n", path)
	}
	for _, path := range data.Removed {
		fmt.Fprintf(p.out, "  - %s (removed)\n", path)
	}
	for _, path := range data.Skipped {
		fmt.Fprintf(p.out, "  ? %s (skipped)\n", path)
	}

	fmt.Fprintln(p.out)
	if data.DryRun {
		fmt.Fprintf(p.out, "%d would be added, %d restored, %d updated, %d unchanged, %d removed\n",
			len(data.Added), len(data.Restored), len(data.Updated), len(data.Unchanged), len(data.Removed))
	} else {
		fmt.Fprintf(p.out, "%d added, %d restored, %d updated, %d unchanged, %d removed\n",
			len(data.Added), len(data.Restored), len(data.Updated), len(data.Unchanged), len(data.Removed))
		if data.Manifest != "" {
			fmt.Fprintf(p.out, "Manifest written to %s\n", data.Manifest)
		}
	}
}

func (p *PlainOutput) ConflictReport(data *ConflictReportData) {
	for _, c := range data.Conflicts {
		fmt.Fprintf(p.out, "  ! %s: %s\n", c.Path, c.Reason)
		if c.Diff != "" {
			fmt.Fprintln(p.out)
			fmt.Fprint(p.out, c.Diff)
			fmt.Fprintln(p.out)
		}
	}
}

func (p *PlainOutput) Success(msg string) {
	fmt.Fprintln(p.out, msg)
}

func (p *PlainOutput) Info(msg string) {
	fmt.Fprintf(p.out, "ℹ %s\n", msg)
}

func (p *PlainOutput) Warning(msg string) {
	fmt.Fprintf(p.out, "Warning: %s\n", msg)
}

func (p *PlainOutput) Error(msg string) {
	fmt.Fprintf(p.err, "Error: %s\n", msg)
}

func (p *PlainOutput) Raw(text string) {
	fmt.Fprintln(p.out, text)
}
