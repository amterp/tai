package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/config"
)

// appendToTargetFile simulates a post_install hook post-processing a managed
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

	changed, err := installer.Rebaseline(adapter, result)
	if err != nil {
		t.Fatalf("Rebaseline failed: %v", err)
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

	changed, err := installer.Rebaseline(adapter, result)
	if err != nil {
		t.Fatalf("Rebaseline failed: %v", err)
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

	changed, err := installer.Rebaseline(adapter, result)
	if err != nil {
		t.Fatalf("Rebaseline failed: %v", err)
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

	changed, err := installer.Rebaseline(adapter, result)
	if err != nil {
		t.Fatalf("Rebaseline failed: %v", err)
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

	changed, err := installer.Rebaseline(adapter, skipped)
	if err != nil {
		t.Fatalf("Rebaseline failed: %v", err)
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

	changed, err := installer.Rebaseline(adapter, result)
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
