package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlainOutput_Header(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.Header("Sources:")

	want := "Sources:\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPlainOutput_List(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.List("Sources:", []ListItem{
		{Name: "local", Description: "local", Extra: "./assets"},
	})

	got := buf.String()
	if !strings.HasPrefix(got, "Sources:\n") {
		t.Errorf("expected to start with 'Sources:\\n', got %q", got)
	}
	if !strings.Contains(got, "local") {
		t.Errorf("expected to contain 'local', got %q", got)
	}
	if !strings.Contains(got, "./assets") {
		t.Errorf("expected to contain './assets', got %q", got)
	}
}

func TestPlainOutput_ListWithDescriptionOnly(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.List("Bundles:", []ListItem{
		{Name: "core", Description: "Core development tools"},
	})

	got := buf.String()
	if !strings.Contains(got, "core") {
		t.Errorf("expected to contain 'core', got %q", got)
	}
	if !strings.Contains(got, "Core development tools") {
		t.Errorf("expected to contain description, got %q", got)
	}
}

func TestPlainOutput_ListWithNameOnly(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.List("Assets:", []ListItem{
		{Name: "personas/senior-swe.md"},
	})

	got := buf.String()
	if !strings.Contains(got, "personas/senior-swe.md") {
		t.Errorf("expected to contain 'personas/senior-swe.md', got %q", got)
	}
}

func TestPlainOutput_KeyValue(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.KeyValue("Active config", "/path/to/config")

	want := "Active config: /path/to/config\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPlainOutput_KeyValueBlock(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.KeyValueBlock([]KeyValuePair{
		{Key: "Profile", Value: "dev"},
		{Key: "Target", Value: "claude"},
	})

	got := buf.String()
	if !strings.Contains(got, "Profile:") {
		t.Errorf("expected to contain 'Profile:', got %q", got)
	}
	if !strings.Contains(got, "Target:") {
		t.Errorf("expected to contain 'Target:', got %q", got)
	}
	if !strings.Contains(got, "dev") {
		t.Errorf("expected to contain 'dev', got %q", got)
	}
	if !strings.Contains(got, "claude") {
		t.Errorf("expected to contain 'claude', got %q", got)
	}
}

func TestPlainOutput_InstallResult(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.InstallResult(&InstallResultData{
		Added:   []string{"personas/senior-swe.md"},
		Updated: []string{"skills/commit.md"},
		Removed: []string{"docs/old.md"},
		Skipped: []string{"docs/conflict.md"},
		DryRun:  false,
	})

	got := buf.String()
	if !strings.Contains(got, "+ personas/senior-swe.md (new)") {
		t.Errorf("expected added item, got %q", got)
	}
	if !strings.Contains(got, "~ skills/commit.md (updated)") {
		t.Errorf("expected updated item, got %q", got)
	}
	if !strings.Contains(got, "- docs/old.md (removed)") {
		t.Errorf("expected removed item, got %q", got)
	}
	if !strings.Contains(got, "? docs/conflict.md (skipped)") {
		t.Errorf("expected skipped item, got %q", got)
	}
	if !strings.Contains(got, "1 added, 0 restored, 1 updated, 0 unchanged, 1 removed") {
		t.Errorf("expected summary, got %q", got)
	}
}

func TestPlainOutput_InstallResultWithUnchanged(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.InstallResult(&InstallResultData{
		Added:     []string{"personas/new.md"},
		Updated:   []string{"skills/changed.md"},
		Unchanged: []string{"docs/same.md", "personas/stable.md"},
		Removed:   []string{"docs/old.md"},
		DryRun:    false,
	})

	got := buf.String()
	if !strings.Contains(got, "+ personas/new.md (new)") {
		t.Errorf("expected added item, got %q", got)
	}
	if !strings.Contains(got, "~ skills/changed.md (updated)") {
		t.Errorf("expected updated item, got %q", got)
	}
	if !strings.Contains(got, "= docs/same.md (unchanged)") {
		t.Errorf("expected unchanged item docs/same.md, got %q", got)
	}
	if !strings.Contains(got, "= personas/stable.md (unchanged)") {
		t.Errorf("expected unchanged item personas/stable.md, got %q", got)
	}
	if !strings.Contains(got, "- docs/old.md (removed)") {
		t.Errorf("expected removed item, got %q", got)
	}
	if !strings.Contains(got, "1 added, 0 restored, 1 updated, 2 unchanged, 1 removed") {
		t.Errorf("expected summary with unchanged count, got %q", got)
	}
}

func TestPlainOutput_InstallResultWithRestored(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.InstallResult(&InstallResultData{
		Restored:  []string{"skills/parallel-work"},
		Unchanged: []string{"docs/same.md"},
	})

	got := buf.String()
	if !strings.Contains(got, "+ skills/parallel-work (restored)") {
		t.Errorf("expected restored item, got %q", got)
	}
	if !strings.Contains(got, "0 added, 1 restored, 0 updated, 1 unchanged, 0 removed") {
		t.Errorf("expected summary with restored count, got %q", got)
	}
}

func TestPlainOutput_InstallResultDryRun(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.InstallResult(&InstallResultData{
		Added:  []string{"personas/senior-swe.md"},
		DryRun: true,
	})

	got := buf.String()
	if !strings.Contains(got, "would be added") {
		t.Errorf("expected dry-run language, got %q", got)
	}
}

func TestPlainOutput_InstallResultDryRunWithUnchanged(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.InstallResult(&InstallResultData{
		Updated:   []string{"skills/changed.md"},
		Unchanged: []string{"docs/same.md"},
		DryRun:    true,
	})

	got := buf.String()
	if !strings.Contains(got, "0 would be added, 0 restored, 1 updated, 1 unchanged, 0 removed") {
		t.Errorf("expected dry-run summary with unchanged, got %q", got)
	}
}

func TestPlainOutput_Messages(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	out := NewPlain(&outBuf, &errBuf)

	out.Success("Profile installed")
	if !strings.Contains(outBuf.String(), "Profile installed") {
		t.Errorf("expected success message, got %q", outBuf.String())
	}

	outBuf.Reset()
	out.Info("Processing files")
	if !strings.Contains(outBuf.String(), "Processing files") {
		t.Errorf("expected info message, got %q", outBuf.String())
	}

	outBuf.Reset()
	out.Warning("File conflict detected")
	if !strings.Contains(outBuf.String(), "Warning:") {
		t.Errorf("expected warning prefix, got %q", outBuf.String())
	}

	out.Error("Installation failed")
	if !strings.Contains(errBuf.String(), "Error:") {
		t.Errorf("expected error prefix in stderr, got %q", errBuf.String())
	}
}

func TestPlainOutput_Raw(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.Raw("/path/to/asset.md")

	want := "/path/to/asset.md\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPlainOutput_Blank(t *testing.T) {
	var buf bytes.Buffer
	out := NewPlain(&buf, &buf)

	out.Blank()

	want := "\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStyledOutput_Blank(t *testing.T) {
	var buf bytes.Buffer
	out := NewStyled(&buf, &buf)

	out.Blank()

	want := "\n"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStyledOutput_Header(t *testing.T) {
	var buf bytes.Buffer
	out := NewStyled(&buf, &buf)

	out.Header("Sources:")

	got := buf.String()
	// Styled output should contain the text (may have ANSI codes)
	if !strings.Contains(got, "Sources:") {
		t.Errorf("expected to contain 'Sources:', got %q", got)
	}
}

func TestStyledOutput_List(t *testing.T) {
	var buf bytes.Buffer
	out := NewStyled(&buf, &buf)

	out.List("Sources:", []ListItem{
		{Name: "local", Description: "local", Extra: "./assets"},
	})

	got := buf.String()
	if !strings.Contains(got, "Sources:") {
		t.Errorf("expected to contain 'Sources:', got %q", got)
	}
	if !strings.Contains(got, "local") {
		t.Errorf("expected to contain 'local', got %q", got)
	}
}

func TestStyledOutput_InstallResult(t *testing.T) {
	var buf bytes.Buffer
	out := NewStyled(&buf, &buf)

	out.InstallResult(&InstallResultData{
		Added:   []string{"personas/senior-swe.md"},
		Updated: []string{"skills/commit.md"},
		DryRun:  false,
	})

	got := buf.String()
	if !strings.Contains(got, "personas/senior-swe.md") {
		t.Errorf("expected added item, got %q", got)
	}
	if !strings.Contains(got, "skills/commit.md") {
		t.Errorf("expected updated item, got %q", got)
	}
}

func TestStyledOutput_InstallResultWithUnchanged(t *testing.T) {
	var buf bytes.Buffer
	out := NewStyled(&buf, &buf)

	out.InstallResult(&InstallResultData{
		Added:     []string{"personas/new.md"},
		Updated:   []string{"skills/changed.md"},
		Unchanged: []string{"docs/same.md"},
		Removed:   []string{"docs/old.md"},
		DryRun:    false,
	})

	got := buf.String()
	if !strings.Contains(got, "personas/new.md") {
		t.Errorf("expected added item, got %q", got)
	}
	if !strings.Contains(got, "skills/changed.md") {
		t.Errorf("expected updated item, got %q", got)
	}
	if !strings.Contains(got, "docs/same.md") {
		t.Errorf("expected unchanged item, got %q", got)
	}
	if !strings.Contains(got, "(unchanged)") {
		t.Errorf("expected '(unchanged)' label, got %q", got)
	}
	if !strings.Contains(got, "docs/old.md") {
		t.Errorf("expected removed item, got %q", got)
	}
	// Summary should include unchanged count
	if !strings.Contains(got, "1 added, 0 restored, 1 updated, 1 unchanged, 1 removed") {
		t.Errorf("expected summary with unchanged, got %q", got)
	}
}

func TestStyledOutput_Messages(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	out := NewStyled(&outBuf, &errBuf)

	out.Success("Done!")
	if !strings.Contains(outBuf.String(), "Done!") {
		t.Errorf("expected success message, got %q", outBuf.String())
	}

	outBuf.Reset()
	out.Info("Processing")
	if !strings.Contains(outBuf.String(), "Processing") {
		t.Errorf("expected info message, got %q", outBuf.String())
	}

	outBuf.Reset()
	out.Warning("Conflict")
	if !strings.Contains(outBuf.String(), "Conflict") {
		t.Errorf("expected warning message, got %q", outBuf.String())
	}

	out.Error("Failed")
	if !strings.Contains(errBuf.String(), "Failed") {
		t.Errorf("expected error message in stderr, got %q", errBuf.String())
	}
}

func TestNew_NonTTY(t *testing.T) {
	var buf bytes.Buffer
	out := New(Options{Writer: &buf, ErrWriter: &buf})

	// Non-TTY should return PlainOutput
	_, isPlain := out.(*PlainOutput)
	if !isPlain {
		t.Errorf("expected PlainOutput for non-TTY, got %T", out)
	}
}

func TestNew_NoColor(t *testing.T) {
	var buf bytes.Buffer
	out := New(Options{Writer: &buf, ErrWriter: &buf, NoColor: true})

	// NoColor should return PlainOutput
	_, isPlain := out.(*PlainOutput)
	if !isPlain {
		t.Errorf("expected PlainOutput with NoColor, got %T", out)
	}
}
