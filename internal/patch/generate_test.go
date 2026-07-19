package patch

import (
	"strings"
	"testing"
)

// roundTrip asserts the generated edits reproduce current from baseline.
func roundTrip(t *testing.T, baseline, current string) []Edit {
	t.Helper()
	edits, err := GenerateEdits([]byte(baseline), []byte(current))
	if err != nil {
		t.Fatalf("GenerateEdits: %v", err)
	}
	got, applyErr := Apply([]byte(baseline), &Patch{Version: 1, File: "f", Name: "gen", Edits: edits})
	if applyErr != nil {
		t.Fatalf("generated edits do not apply: %v", applyErr)
	}
	if string(got) != current {
		t.Fatalf("round trip = %q, want %q", got, current)
	}
	return edits
}

func TestGenerateEditsReplacement(t *testing.T) {
	edits := roundTrip(t,
		"one\nAlways install with npm.\nthree\n",
		"one\nAlways install with pnpm.\nthree\n")
	if len(edits) != 1 {
		t.Errorf("len(edits) = %d, want 1", len(edits))
	}
}

func TestGenerateEditsAppendNeedsNoManualAnchor(t *testing.T) {
	// The case that is awkward to hand-write: a pure addition. Generation picks
	// the anchor, so the user never repeats text in both find and replace.
	edits := roundTrip(t,
		"# Skill\n\nBody.\n",
		"# Skill\n\nBody.\n\n## My Addition\n\nUses my tooling.\n")
	if len(edits) != 1 {
		t.Fatalf("len(edits) = %d, want 1", len(edits))
	}
	if edits[0].Find == "" {
		t.Error("find must be non-empty for a pure insertion")
	}
	if !strings.Contains(edits[0].Replace, "My Addition") {
		t.Errorf("replace = %q, want it to add the new section", edits[0].Replace)
	}
}

func TestGenerateEditsDeletion(t *testing.T) {
	roundTrip(t, "keep\nDELETE ME\nkeep two\n", "keep\nkeep two\n")
}

func TestGenerateEditsMultipleHunks(t *testing.T) {
	edits := roundTrip(t,
		"alpha\nfiller one\nfiller two\nfiller three\nomega\n",
		"ALPHA\nfiller one\nfiller two\nfiller three\nOMEGA\n")
	if len(edits) != 2 {
		t.Errorf("len(edits) = %d, want 2 separate hunks", len(edits))
	}
}

func TestGenerateEditsWidensContextForRepeatedLines(t *testing.T) {
	// The changed line is not unique on its own, so generation must widen until
	// the anchor is unambiguous rather than emit a find that would be rejected.
	baseline := "dup\nunique-a\ndup\nunique-b\ndup\n"
	current := "dup\nunique-a\nCHANGED\nunique-b\ndup\n"
	edits := roundTrip(t, baseline, current)
	if strings.Count(baseline, edits[0].Find) != 1 {
		t.Errorf("find %q is not unique in the baseline", edits[0].Find)
	}
}

func TestGenerateEditsIdenticalContentIsAnError(t *testing.T) {
	_, err := GenerateEdits([]byte("same\n"), []byte("same\n"))
	if err == nil {
		t.Fatal("expected an error when nothing changed")
	}
	if !strings.Contains(err.Error(), "no changes") {
		t.Errorf("err = %v, want it to say there is nothing to record", err)
	}
}

func TestGenerateEditsUnanchorableChangeFails(t *testing.T) {
	// Every line identical: no amount of context makes a hunk unique, so this
	// must fail loudly rather than emit an edit that would misapply.
	baseline := strings.Repeat("x\n", 20)
	current := strings.Repeat("x\n", 10) + "y\n" + strings.Repeat("x\n", 9)
	if _, err := GenerateEdits([]byte(baseline), []byte(current)); err == nil {
		t.Fatal("expected an error for content with no unique anchor")
	}
}

func TestGenerateEditsNoTrailingNewline(t *testing.T) {
	roundTrip(t, "line one\nlast line no newline", "line one\nlast line CHANGED")
}
