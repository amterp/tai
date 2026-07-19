package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/output"
	"github.com/amterp/tai/internal/patch"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/state"
)

// patchRef pairs an inspection with the source it came from, since patch names
// are only unique within a source.
type patchRef struct {
	source *source.LoadedSource
	insp   *patch.Inspection
}

func (r patchRef) qualifiedName() string {
	return r.source.Name + ":" + r.insp.Patch.Name
}

// inspectAllPatches gathers every declared patch across all sources, in source
// order then set order.
func inspectAllPatches(s *state.State) []patchRef {
	var names []string
	for name, src := range s.Sources {
		if src.HasPatches() {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var refs []patchRef
	for _, name := range names {
		src := s.Sources[name]
		patches, err := s.LoadPatches(src)
		if err != nil {
			fatal("%v", err)
		}
		inspections, err := src.InspectPatches(patches)
		if err != nil {
			fatal("inspecting patches for source %q: %v", name, err)
		}
		for _, insp := range inspections {
			refs = append(refs, patchRef{source: src, insp: insp})
		}
	}
	return refs
}

// findPatch resolves a user-supplied name. A bare name is accepted when it is
// unambiguous; `source:name` always is.
func findPatch(s *state.State, name string) patchRef {
	refs := inspectAllPatches(s)

	var matches []patchRef
	for _, r := range refs {
		if r.qualifiedName() == name || r.insp.Patch.Name == name {
			matches = append(matches, r)
		}
	}

	switch len(matches) {
	case 0:
		fatal("unknown patch %q (see 'tai patch list')", name)
	case 1:
		return matches[0]
	}

	var qualified []string
	for _, m := range matches {
		qualified = append(qualified, m.qualifiedName())
	}
	fatal("patch %q is ambiguous - it exists in several sources: %s", name, strings.Join(qualified, ", "))
	return patchRef{}
}

func runPatchList() {
	s := loadState()
	requireActiveConfig(s)

	refs := inspectAllPatches(s)
	if len(refs) == 0 {
		out.Info("No patches defined. Declare one with a source's 'patches:' field.")
		return
	}

	items := make([]output.ListItem, 0, len(refs))
	for _, r := range refs {
		status := r.insp.Status.String()
		if r.insp.Status == patch.StatusBroken && r.insp.ApplyErr != nil {
			status = "BROKEN: " + r.insp.ApplyErr.Error()
		}

		desc := r.insp.Patch.FileRef()
		if d := r.insp.Patch.Description; d != "" {
			desc = d + " (" + desc + ")"
		}
		items = append(items, output.ListItem{Name: r.qualifiedName(), Description: desc, Extra: status})
	}
	out.List("Patches:", items)
}

func runPatchDiff(name string) {
	s := loadState()
	requireActiveConfig(s)

	r := findPatch(s, name)
	p := r.insp.Patch

	if r.insp.Status == patch.StatusSourceUnfetched {
		fatal("source %q has not been fetched yet; run 'tai refresh' first", p.SourceName)
	}
	if r.insp.Status == patch.StatusFileMissing {
		fatal("patch %q targets %s, which the source does not carry", p.Name, p.FileRef())
	}
	if !r.insp.HasBlessed {
		// Never suggest blessing a broken patch: bless refuses those, so the
		// two commands would point at each other while the user is mid-repair.
		if r.insp.Status == patch.StatusBroken {
			fatal("patch %q was never blessed, so there is no snapshot to compare against - "+
				"tai cannot show what moved upstream.\nRepair it instead:  tai patch workspace %s",
				p.Name, p.SourceName)
		}
		fatal("patch %q has no blessed snapshot to compare against - run 'tai patch bless %s' first", p.Name, name)
	}
	if bytes.Equal(r.insp.Blessed, r.insp.Pristine) {
		if r.insp.Status == patch.StatusBroken {
			// Upstream is byte-identical to what was reviewed, so the patch is
			// what moved. Saying only "no upstream changes" would read as "all
			// is well" to someone who just watched an install fail.
			out.Warning(fmt.Sprintf(
				"%s is unchanged since '%s' was blessed, so the patch itself no longer fits it: %s\n"+
					"Repair it:  tai patch workspace %s",
				p.FileRef(), p.Name, r.insp.ApplyErr.Error(), p.SourceName))
			return
		}
		out.Info(fmt.Sprintf("No upstream changes since '%s' was blessed.", p.Name))
		return
	}

	// Reuse the conflict differ: the question has the same shape - what changed
	// under me since I last looked.
	out.Raw(install.GenerateDiff(install.ConflictDetail{
		Path:            p.FileRef(),
		ExistingContent: r.insp.Blessed,
		ResolvedContent: r.insp.Pristine,
	}))
}

func runPatchBless(name string, all bool) {
	s := loadState()
	requireActiveConfig(s)

	if all == (name != "") {
		fatal("specify either a patch name or --all")
	}

	targets := inspectAllPatches(s)
	if !all {
		targets = []patchRef{findPatch(s, name)}
	}
	if len(targets) == 0 {
		out.Info("No patches defined.")
		return
	}

	blessed := 0
	for _, r := range targets {
		p := r.insp.Patch

		// Blessing means "I reviewed this upstream and the patch is right for
		// it". Recording a snapshot the patch cannot even apply to would bake
		// in a broken state and silence the drift warning that would prompt a
		// fix.
		if reason := blessBlocker(r.insp); reason != "" {
			if all {
				out.Warning(fmt.Sprintf("skipping %s: %s", r.qualifiedName(), reason))
				continue
			}
			fatal("cannot bless %s: %s", r.qualifiedName(), reason)
		}

		if r.insp.HasBlessed && bytes.Equal(r.insp.Blessed, r.insp.Pristine) {
			if !all {
				out.Info(fmt.Sprintf("Patch '%s' is already blessed against current upstream.", p.Name))
			}
			continue
		}

		if err := os.WriteFile(p.BlessedPath, r.insp.Pristine, 0644); err != nil {
			fatal("writing snapshot for %q: %v", p.Name, err)
		}
		out.Success(fmt.Sprintf("Blessed '%s' against current %s", r.qualifiedName(), p.FileRef()))
		blessed++
	}

	if blessed > 0 {
		out.Info("Commit the .blessed file(s) so drift detection travels with your config repo.")
	}
}

// blessBlocker returns why a patch cannot be blessed, or "" when it can.
func blessBlocker(insp *patch.Inspection) string {
	switch insp.Status {
	case patch.StatusBroken:
		return "it does not apply to current upstream - fix the patch first (" + insp.ApplyErr.Error() + ")"
	case patch.StatusSourceUnfetched:
		return "its source has not been fetched - run 'tai refresh' first"
	case patch.StatusFileMissing:
		return "it targets " + insp.Patch.FileRef() + ", which the source does not carry"
	}
	return ""
}

// warnAboutPatchDrift reports patches that still apply but whose upstream has
// moved since it was reviewed. The patch working is not the same as it still
// being right: upstream may have changed something the patch has an opinion
// about, somewhere the patch does not touch.
func warnAboutPatchDrift(s *state.State) {
	for _, r := range inspectAllPatches(s) {
		p := r.insp.Patch
		switch r.insp.Status {
		case patch.StatusDrifted:
			out.Warning(fmt.Sprintf(
				"upstream file for patch '%s' changed since it was blessed\n"+
					"  file: %s\n"+
					"  The patch still applies cleanly; installing the patched content.\n"+
					"  Review what changed:  tai patch diff %s\n"+
					"  Accept it:            tai patch bless %s",
				p.Name, p.FileRef(), r.qualifiedName(), r.qualifiedName()))
		case patch.StatusUnblessed:
			out.Info(fmt.Sprintf(
				"Patch '%s' is not blessed - tai cannot tell you when %s changes upstream. Run 'tai patch bless %s'.",
				p.Name, p.FileRef(), r.qualifiedName()))
		}
	}
}

func runPatchWorkspace(sourceName string) {
	s := loadState()
	requireActiveConfig(s)

	src, ok := s.Sources[sourceName]
	if !ok {
		fatal("unknown source %q", sourceName)
	}
	if !src.HasPatches() {
		fatal("source %q declares no 'patches:' directory", sourceName)
	}

	patches, err := s.LoadPatches(src)
	if err != nil {
		fatal("%v", err)
	}

	path, skipped, err := s.BuildPatchWorkspace(src, patches)
	if err != nil {
		fatal("%v", err)
	}

	out.Success("Workspace ready: " + path)
	if len(skipped) > 0 {
		out.Warning("These patches could not be applied and are NOT reflected in the workspace:\n  " +
			strings.Join(skipped, "\n  ") + "\nRe-make their changes by hand, then regenerate them.")
	}
	out.Info("Edit files there, then run:  tai patch regenerate <patch-name>")
}

func runPatchRegenerate(name string) {
	s := loadState()
	requireActiveConfig(s)

	r := findPatch(s, name)
	p := r.insp.Patch

	patches, err := s.LoadPatches(r.source)
	if err != nil {
		fatal("%v", err)
	}

	wsPath, err := s.PatchWorkspacePath(r.source.Name)
	if err != nil {
		fatal("%v", err)
	}
	edited, err := os.ReadFile(filepath.Join(wsPath, filepath.FromSlash(p.File)))
	if err != nil {
		fatal("reading %s from the workspace: %v\nRun 'tai patch workspace %s' first.", p.File, err, r.source.Name)
	}

	// Measured against pristine plus every *other* patch on this file, so
	// regenerating one patch can never absorb a sibling's changes.
	baseline, err := r.source.RegenerationBaseline(patches, p)
	if err != nil {
		fatal("%v", err)
	}

	edits, err := patch.GenerateEdits(baseline, edited)
	if err != nil {
		fatal("%v", err)
	}

	p.Edits = edits
	if err := writePatchFile(p); err != nil {
		fatal("writing %s: %v", p.PatchPath, err)
	}

	out.Success(fmt.Sprintf("Regenerated '%s' with %d edit(s): %s", p.Name, len(edits), p.PatchPath))
	out.Info("Review the result, then run:  tai patch bless " + r.qualifiedName())
}

// writePatchFile rewrites a patch file from its in-memory form.
//
// Built as a node tree rather than marshalled directly so multi-line find and
// replace values use literal block style. A regenerated patch is meant to be
// read and reviewed; the same content as escaped one-line strings is not.
func writePatchFile(p *patch.Patch) error {
	doc := &yaml.Node{Kind: yaml.MappingNode}

	addScalar := func(key string, value *yaml.Node) {
		doc.Content = append(doc.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
	}

	addScalar("version", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(p.Version)})
	addScalar("file", &yaml.Node{Kind: yaml.ScalarNode, Value: p.File})
	if p.Description != "" {
		addScalar("description", &yaml.Node{Kind: yaml.ScalarNode, Value: p.Description})
	}

	editsNode := &yaml.Node{Kind: yaml.SequenceNode}
	for _, e := range p.Edits {
		item := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "find"}, scalarPreferringBlock(e.Find),
			{Kind: yaml.ScalarNode, Value: "replace"}, scalarPreferringBlock(e.Replace),
		}}
		editsNode.Content = append(editsNode.Content, item)
	}
	addScalar("edits", editsNode)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(p.PatchPath, buf.Bytes(), 0644)
}

// scalarPreferringBlock renders multi-line text as a literal block so the
// patch file stays readable.
func scalarPreferringBlock(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Value: s}
	if strings.Contains(s, "\n") {
		n.Style = yaml.LiteralStyle
	}
	return n
}
