package patch

import (
	"fmt"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// contextSteps is how far edit generation will widen a hunk's surrounding
// context in search of a unique anchor. Growth is superlinear because the
// common case needs one or two lines and the rare case needs a lot.
var contextSteps = []int{0, 1, 2, 3, 5, 8, 13, 21, 34, 55}

// GenerateEdits derives search/replace edits that transform baseline into
// current.
//
// This is what makes hand-authoring unnecessary: you edit the file normally
// and let tai work out the edits, so an append doesn't need an anchor repeated
// verbatim in both find and replace.
//
// Each edit's find is widened with surrounding context until it appears exactly
// once in baseline, matching the rule Apply enforces. The generated set is then
// applied to baseline and checked against current, so a patch that regenerates
// successfully is known to reproduce the edit rather than merely look plausible.
func GenerateEdits(baseline, current []byte) ([]Edit, error) {
	if string(baseline) == string(current) {
		return nil, fmt.Errorf("no changes to record: the file is identical to its unpatched content")
	}

	baseLines := splitLines(string(baseline))
	curLines := splitLines(string(current))

	opcodes := difflib.NewMatcher(baseLines, curLines).GetOpCodes()

	edits, err := editsFromOpcodes(string(baseline), baseLines, curLines, opcodes)
	if err != nil {
		return nil, err
	}

	// Generated edits apply in sequence, so an earlier replacement could in
	// principle disturb a later find. Rather than reason about that, apply them
	// and check the result.
	probe := &Patch{Version: 1, File: "generated", Edits: edits, Name: "generated"}
	got, applyErr := Apply(baseline, probe)
	if applyErr != nil {
		return nil, fmt.Errorf("generated edits do not apply cleanly (%s); the change may be too repetitive to anchor uniquely", applyErr.Error())
	}
	if string(got) != string(current) {
		return nil, fmt.Errorf("generated edits did not reproduce the edited file; please report this")
	}

	return edits, nil
}

// editsFromOpcodes turns each run of changed lines into one edit.
func editsFromOpcodes(baseline string, baseLines, curLines []string, opcodes []difflib.OpCode) ([]Edit, error) {
	var edits []Edit

	for i := 0; i < len(opcodes); i++ {
		if opcodes[i].Tag == 'e' {
			continue
		}
		// Merge adjacent non-equal opcodes into a single hunk.
		start := i
		for i+1 < len(opcodes) && opcodes[i+1].Tag != 'e' {
			i++
		}

		hunk := struct{ b1, b2, c1, c2 int }{
			b1: opcodes[start].I1, b2: opcodes[i].I2,
			c1: opcodes[start].J1, c2: opcodes[i].J2,
		}

		edit, err := uniqueEdit(baseline, baseLines, curLines, hunk.b1, hunk.b2, hunk.c1, hunk.c2)
		if err != nil {
			return nil, err
		}
		edits = append(edits, edit)
	}

	return edits, nil
}

// uniqueEdit widens context around a hunk until its find text is unique.
func uniqueEdit(baseline string, baseLines, curLines []string, b1, b2, c1, c2 int) (Edit, error) {
	for _, ctx := range contextSteps {
		lo := max(0, b1-ctx)
		hi := min(len(baseLines), b2+ctx)
		clo := max(0, c1-ctx)
		chi := min(len(curLines), c2+ctx)

		find := strings.Join(baseLines[lo:hi], "")
		// A pure insertion has an empty find at zero context, and an empty find
		// is not a valid edit - widen until there is something to anchor to.
		if find == "" {
			continue
		}
		if strings.Count(baseline, find) == 1 {
			return Edit{Find: find, Replace: strings.Join(curLines[clo:chi], "")}, nil
		}
	}

	// An empty baseline has nothing to anchor against at all, which is a
	// different problem from ambiguity and would send someone hunting for
	// repetition that isn't there.
	if len(baseLines) == 0 {
		return Edit{}, fmt.Errorf("the unpatched file is empty, so there is no text for an edit to anchor to - " +
			"a patch edits existing content and cannot create a file from nothing")
	}
	return Edit{}, fmt.Errorf("could not find a unique anchor for a change near line %d - the surrounding content repeats too often to patch reliably", b1+1)
}

// splitLines splits into newline-terminated lines that rejoin exactly.
//
// difflib.SplitLines is not usable here: it appends a newline to the final
// element, so joining its output adds a trailing blank line and every
// generated edit would silently corrupt the end of the file.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
