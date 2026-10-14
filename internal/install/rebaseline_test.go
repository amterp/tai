package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/adapter"
	"github.com/amterp/tai/internal/config"
)

// rebaselineAfterSuccess runs the tracker for a single step that succeeded.
func rebaselineAfterSuccess(installer *Installer, adap adapter.Adapter, result *InstallResult) ([]string, error) {
	changes, err := installer.TrackPostInstall(adap, result).AfterStep(true)
	return changes.Baselined, err
}

// appendToTargetFile simulates a post_install step post-processing a managed
// file - memri's real hook appends an @import line to CLAUDE.md.
func appendToTargetFile(t *testing.T, path, extra string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(content, []byte(extra)...), 0644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestRebaselineAcceptsHookEditsToManagedFile(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test Persona")
	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	installed := filepath.Join(env.TargetDir, "output-styles", "test.md")
	appendToTargetFile(t, installed, "\n@import ~/.memri/nudge.md\n")

	changed, err := rebaselineAfterSuccess(installer, adapter, result)
	if err != nil {
		t.Fatalf("rebaseline failed: %v", err)
	}
	if len(changed) != 1 || changed[0] != "output-styles/test.md" {
		t.Fatalf("changed = %v, want [output-styles/test.md]", changed)
	}

	// The persisted manifest must describe what is on disk, not what was written.
	manifest, err := installer.Status("claude")
	if err != nil || manifest == nil {
		t.Fatalf("loading manifest: %v", err)
	}
	onDisk, err := ComputeFileChecksum(installed)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Files[0].Checksum != onDisk {
		t.Errorf("manifest checksum = %q, want on-disk %q", manifest.Files[0].Checksum, onDisk)
	}

	// The whole point: reinstalling must not report the hook's own edit as a conflict.
	if _, err := installer.Install(adapter, "test-profile", assets, InstallOptions{}); err != nil {
		t.Fatalf("second Install should not conflict after rebaseline: %v", err)
	}
}

func TestRebaselineIgnoresUntouchedFiles(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test Persona")
	env.SetupAsset("delegates", "helper.md", "---\ntai-schema-version: 1\nname: Helper\n---\n\n# Helper Agent")
	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
		{Type: "delegate", Name: "helper.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "helper.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	appendToTargetFile(t, filepath.Join(env.TargetDir, "agents", "helper.md"), "\nextra\n")

	changed, err := rebaselineAfterSuccess(installer, adapter, result)
	if err != nil {
		t.Fatalf("rebaseline failed: %v", err)
	}
	if len(changed) != 1 || changed[0] != "agents/helper.md" {
		t.Errorf("changed = %v, want only [agents/helper.md]", changed)
	}
}

func TestRebaselineNoChangesLeavesManifestAlone(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test Persona")
	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	before := result.Manifest.Files[0].Checksum

	changed, err := rebaselineAfterSuccess(installer, adapter, result)
	if err != nil {
		t.Fatalf("rebaseline failed: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("changed = %v, want none for a hook that touched nothing", changed)
	}
	if result.Manifest.Files[0].Checksum != before {
		t.Error("checksum should be untouched when nothing changed on disk")
	}
}

func TestRebaselineDirAsset(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillMd := "---\ntai-schema-version: 1\nname: My Skill\ndescription: A helpful skill\n---\n\n# My Skill\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.sh"), []byte("#!/bin/bash\necho hello"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "skill", Name: "my-skill", SourceName: "local", SourcePath: skillDir, IsDir: true},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	appendToTargetFile(t, filepath.Join(env.TargetDir, "skills", "my-skill", "helper.sh"), "\necho patched\n")

	changed, err := rebaselineAfterSuccess(installer, adapter, result)
	if err != nil {
		t.Fatalf("rebaseline failed: %v", err)
	}
	if len(changed) != 1 || changed[0] != "skills/my-skill" {
		t.Fatalf("changed = %v, want [skills/my-skill]", changed)
	}

	if _, err := installer.Install(adapter, "test-profile", assets, InstallOptions{}); err != nil {
		t.Fatalf("second Install should not conflict after rebaseline: %v", err)
	}
}

// A file the user hand-edited and then deliberately skipped must keep its stale
// checksum, so it keeps conflicting until they resolve it. Rebaselining it would
// silently discard that decision - the exact failure mode that made normalizing
// --force unacceptable in the first place.
func TestRebaselinePreservesSkippedConflicts(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test Persona")
	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	first, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	writtenChecksum := first.Manifest.Files[0].Checksum

	// Hand-edit, then install with --skip-conflicts to leave it alone.
	appendToTargetFile(t, filepath.Join(env.TargetDir, "output-styles", "test.md"), "\nmy own notes\n")

	skipped, err := installer.Install(adapter, "test-profile", assets, InstallOptions{SkipConflicts: true})
	if err != nil {
		t.Fatalf("Install with SkipConflicts failed: %v", err)
	}
	if len(skipped.Skipped) != 1 {
		t.Fatalf("Skipped = %v, want the hand-edited file", skipped.Skipped)
	}

	changed, err := rebaselineAfterSuccess(installer, adapter, skipped)
	if err != nil {
		t.Fatalf("rebaseline failed: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("changed = %v, want none - a skipped conflict must not be baselined", changed)
	}

	manifest, err := installer.Status("claude")
	if err != nil || manifest == nil {
		t.Fatalf("loading manifest: %v", err)
	}
	if manifest.Files[0].Checksum != writtenChecksum {
		t.Error("skipped entry's checksum changed; the hand edit was silently accepted")
	}

	// The hand edit must still be reported.
	if _, err := installer.Install(adapter, "test-profile", assets, InstallOptions{}); err == nil {
		t.Error("hand-edited file should still conflict after a rebaseline")
	} else if _, ok := err.(*ConflictsError); !ok {
		t.Errorf("want ConflictsError, got %v", err)
	}
}

func TestRebaselineToleratesFileRemovedByHook(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test Persona")
	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	before := result.Manifest.Files[0].Checksum

	if err := os.Remove(filepath.Join(env.TargetDir, "output-styles", "test.md")); err != nil {
		t.Fatal(err)
	}

	changed, err := rebaselineAfterSuccess(installer, adapter, result)
	if err != nil {
		t.Fatalf("Rebaseline must not fail on a file the hook removed: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("changed = %v, want none", changed)
	}
	if result.Manifest.Files[0].Checksum != before {
		t.Error("entry for a removed file should keep its checksum")
	}

	// A missing file is not a conflict - the next install just puts it back.
	next, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("reinstall after hook removed the file: %v", err)
	}
	if !env.TargetFileExists("output-styles/test.md") {
		t.Error("file should have been reinstalled")
	}
	_ = next
}

// twoPersonas installs two managed files, so a test can have different steps
// edit different files.
func twoPersonas(t *testing.T) (*Installer, adapter.Adapter, []*config.Asset, *InstallResult, string, string) {
	t.Helper()
	env, installer, adap := setupInstallTest(t)

	env.SetupAsset("personas", "a.md", "---\ntai-schema-version: 1\nname: A\n---\n\n# A")
	env.SetupAsset("personas", "b.md", "---\ntai-schema-version: 1\nname: B\n---\n\n# B")
	assets := []*config.Asset{
		{Type: "persona", Name: "a.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "a.md")},
		{Type: "persona", Name: "b.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "b.md")},
	}
	result, err := installer.Install(adap, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	return installer, adap, assets, result,
		filepath.Join(env.TargetDir, "output-styles", "a.md"),
		filepath.Join(env.TargetDir, "output-styles", "b.md")
}

func checksumOf(t *testing.T, m *Manifest, path string) string {
	t.Helper()
	for _, f := range m.Files {
		if f.Path == path {
			return f.Checksum
		}
	}
	t.Fatalf("manifest has no entry for %s", path)
	return ""
}

// conflictPaths reinstalls and returns the paths reported as conflicts.
func conflictPaths(t *testing.T, installer *Installer, adap adapter.Adapter, assets []*config.Asset) []string {
	t.Helper()
	_, err := installer.Install(adap, "test-profile", assets, InstallOptions{})
	if err == nil {
		return nil
	}
	conflicts, ok := err.(*ConflictsError)
	if !ok {
		t.Fatalf("want ConflictsError or nil, got %v", err)
	}
	var paths []string
	for _, c := range conflicts.Conflicts {
		paths = append(paths, c.Path)
	}
	return paths
}

func TestRebaselineHoldsFailedStepEdits(t *testing.T) {
	installer, adap, assets, result, a, _ := twoPersonas(t)

	tracker := installer.TrackPostInstall(adap, result)
	appendToTargetFile(t, a, "\npartial edit\n")
	changes, err := tracker.AfterStep(false)
	if err != nil {
		t.Fatalf("AfterStep failed: %v", err)
	}
	if len(changes.Baselined) != 0 || len(changes.Held) != 1 || changes.Held[0] != "output-styles/a.md" {
		t.Fatalf("changes = %+v, want only a.md held", changes)
	}

	if got := conflictPaths(t, installer, adap, assets); len(got) != 1 || got[0] != "output-styles/a.md" {
		t.Errorf("conflicts = %v, want [output-styles/a.md] - a failed step's edit must surface", got)
	}
}

// The reason steps exist: one failing step must not stop a later, unrelated
// step's edits from being baselined.
func TestRebaselineSucceededStepAfterFailedOne(t *testing.T) {
	installer, adap, assets, result, a, b := twoPersonas(t)

	tracker := installer.TrackPostInstall(adap, result)
	appendToTargetFile(t, a, "\npartial edit\n")
	if _, err := tracker.AfterStep(false); err != nil {
		t.Fatalf("AfterStep(false) failed: %v", err)
	}
	appendToTargetFile(t, b, "\n@import ~/.memri/nudge.md\n")
	changes, err := tracker.AfterStep(true)
	if err != nil {
		t.Fatalf("AfterStep(true) failed: %v", err)
	}
	if len(changes.Held) != 0 || len(changes.Baselined) != 1 || changes.Baselined[0] != "output-styles/b.md" {
		t.Fatalf("changes = %+v, want only b.md baselined", changes)
	}

	if got := conflictPaths(t, installer, adap, assets); len(got) != 1 || got[0] != "output-styles/a.md" {
		t.Errorf("conflicts = %v, want only the failed step's [output-styles/a.md]", got)
	}
}

// A later successful step re-touching a file a failed step left half-edited
// must not baseline it: the file's content still includes the partial edit.
func TestRebaselineKeepsFailedStepFileHeld(t *testing.T) {
	installer, adap, assets, result, a, _ := twoPersonas(t)
	recorded := checksumOf(t, result.Manifest, "output-styles/a.md")

	tracker := installer.TrackPostInstall(adap, result)
	appendToTargetFile(t, a, "\npartial edit\n")
	if _, err := tracker.AfterStep(false); err != nil {
		t.Fatalf("AfterStep(false) failed: %v", err)
	}
	appendToTargetFile(t, a, "\nformatted\n")
	changes, err := tracker.AfterStep(true)
	if err != nil {
		t.Fatalf("AfterStep(true) failed: %v", err)
	}
	if len(changes.Baselined) != 0 || len(changes.Held) != 1 {
		t.Fatalf("changes = %+v, want a.md held", changes)
	}
	if checksumOf(t, result.Manifest, "output-styles/a.md") != recorded {
		t.Error("a.md's checksum changed; the failed step's partial edit was baselined")
	}
	if got := conflictPaths(t, installer, adap, assets); len(got) != 1 || got[0] != "output-styles/a.md" {
		t.Errorf("conflicts = %v, want [output-styles/a.md]", got)
	}
}

// Successive successful steps each baseline their own edit to the same file.
func TestRebaselineSuccessiveStepsOnOneFile(t *testing.T) {
	installer, adap, assets, result, a, _ := twoPersonas(t)

	tracker := installer.TrackPostInstall(adap, result)
	for step, line := range []string{"\nfirst\n", "\nsecond\n"} {
		appendToTargetFile(t, a, line)
		changes, err := tracker.AfterStep(true)
		if err != nil {
			t.Fatalf("AfterStep on step %d failed: %v", step, err)
		}
		if len(changes.Baselined) != 1 || changes.Baselined[0] != "output-styles/a.md" {
			t.Fatalf("step %d changes = %+v, want a.md baselined", step, changes)
		}
	}
	if got := conflictPaths(t, installer, adap, assets); len(got) != 0 {
		t.Errorf("conflicts = %v, want none", got)
	}
}
