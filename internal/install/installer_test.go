package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/template"
	"github.com/amterp/tai/internal/testutil"
)

func setupInstallTest(t *testing.T) (*testutil.TestEnv, *Installer, *claude.Adapter) {
	t.Helper()
	env := testutil.NewTestEnv(t)

	manifestStore := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := claude.New(env.TargetDir)

	return env, installer, adapter
}

func TestInstallerInstallSimple(t *testing.T) {
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

	if len(result.Added) != 2 {
		t.Errorf("len(Added) = %d, want 2", len(result.Added))
	}
	if len(result.Updated) != 0 {
		t.Errorf("len(Updated) = %d, want 0", len(result.Updated))
	}

	if !env.TargetFileExists("output-styles/test.md") {
		t.Error("output-styles/test.md not created")
	}
	if !env.TargetFileExists("agents/helper.md") {
		t.Error("agents/helper.md not created")
	}

	// Adapter renders Claude frontmatter (name only, no tai-schema-version)
	content := env.ReadTargetFile("output-styles/test.md")
	expected := "---\nname: Test\n---\n\n# Test Persona"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}

	if result.Manifest == nil {
		t.Fatal("Manifest is nil")
	}
	if result.Manifest.Profile != "test-profile" {
		t.Errorf("Manifest.Profile = %q, want 'test-profile'", result.Manifest.Profile)
	}
}

func TestInstallerInstallDryRun(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test Persona")

	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	if env.TargetFileExists("output-styles/test.md") {
		t.Error("file created during dry run")
	}
}

func TestInstallerInstallUpdate(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Version 1")
	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Version 2")

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	if len(result.Updated) != 1 {
		t.Errorf("len(Updated) = %d, want 1", len(result.Updated))
	}
	if len(result.Unchanged) != 0 {
		t.Errorf("len(Unchanged) = %d, want 0", len(result.Unchanged))
	}

	content := env.ReadTargetFile("output-styles/test.md")
	expected := "---\nname: Test\n---\n\n# Version 2"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerInstallUnchanged(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Same\n---\n\n# Same Content")
	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	result1, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	if len(result1.Added) != 1 {
		t.Errorf("First install: len(Added) = %d, want 1", len(result1.Added))
	}

	result2, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	if len(result2.Unchanged) != 1 {
		t.Errorf("Second install: len(Unchanged) = %d, want 1", len(result2.Unchanged))
	}
	if len(result2.Updated) != 0 {
		t.Errorf("Second install: len(Updated) = %d, want 0", len(result2.Updated))
	}
	if len(result2.Added) != 0 {
		t.Errorf("Second install: len(Added) = %d, want 0", len(result2.Added))
	}
}

func TestInstallerInstallMixedUnchangedAndUpdated(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "unchanged.md", "---\ntai-schema-version: 1\nname: Unchanged\n---\n\n# Will Stay Same")
	env.SetupAsset("personas", "updated.md", "---\ntai-schema-version: 1\nname: Updated\n---\n\n# Version 1")

	assets := []*config.Asset{
		{Type: "persona", Name: "unchanged.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "unchanged.md")},
		{Type: "persona", Name: "updated.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "updated.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	env.SetupAsset("personas", "updated.md", "---\ntai-schema-version: 1\nname: Updated\n---\n\n# Version 2")

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	if len(result.Unchanged) != 1 {
		t.Errorf("len(Unchanged) = %d, want 1", len(result.Unchanged))
	}
	if len(result.Updated) != 1 {
		t.Errorf("len(Updated) = %d, want 1", len(result.Updated))
	}

	if result.Unchanged[0] != "output-styles/unchanged.md" {
		t.Errorf("Unchanged[0] = %q, want 'output-styles/unchanged.md'", result.Unchanged[0])
	}
	if result.Updated[0] != "output-styles/updated.md" {
		t.Errorf("Updated[0] = %q, want 'output-styles/updated.md'", result.Updated[0])
	}
}

func TestInstallerInstallRemoveStale(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "keep.md", "---\ntai-schema-version: 1\nname: Keep\n---\n\n# Keep")
	env.SetupAsset("personas", "remove.md", "---\ntai-schema-version: 1\nname: Remove\n---\n\n# Remove")

	assets := []*config.Asset{
		{Type: "persona", Name: "keep.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "keep.md")},
		{Type: "persona", Name: "remove.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "remove.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	assets = []*config.Asset{
		{Type: "persona", Name: "keep.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "keep.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	if len(result.Removed) != 1 {
		t.Errorf("len(Removed) = %d, want 1", len(result.Removed))
	}

	if env.TargetFileExists("output-styles/remove.md") {
		t.Error("removed file still exists")
	}
	if !env.TargetFileExists("output-styles/keep.md") {
		t.Error("kept file was removed")
	}
}

func TestInstallerInstallWithTemplating(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "templated.md", "---\ntai-schema-version: 1\nname: Templated\n---\n\n# Persona for {{project_name}}")

	assets := []*config.Asset{
		{Type: "persona", Name: "templated.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "templated.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{
		Variables: template.Variables{"project_name": "MyProject"},
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	content := env.ReadTargetFile("output-styles/templated.md")
	expected := "---\nname: Templated\n---\n\n# Persona for MyProject"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerInstallWithExisting(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Create existing file at target
	existingPath := filepath.Join(env.TargetDir, "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existingPath, []byte("# Existing Content"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupAsset("files", "CLAUDE.md", "# Header\n\n{{existing}}\n\n# Footer")

	assets := []*config.Asset{
		{Type: "file", Name: "CLAUDE.md", SourceName: "local", Dest: "",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "CLAUDE.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Manifest.Files) != 1 {
		t.Fatal("expected 1 manifest entry")
	}
	if result.Manifest.Files[0].BackupPath == "" {
		t.Error("BackupPath is empty")
	}

	// Files are copied as-is (after template substitution)
	content := env.ReadTargetFile("CLAUDE.md")
	expected := "# Header\n\n# Existing Content\n\n# Footer"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}

	backupPath := existingPath + ".tai-backup"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file not created")
	}
}

func TestInstallerInstallConflictUnmanaged(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	targetPath := filepath.Join(env.TargetDir, "output-styles", "conflict.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("# Unmanaged"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupAsset("personas", "conflict.md", "---\ntai-schema-version: 1\nname: Conflict\n---\n\n# New Content")

	assets := []*config.Asset{
		{Type: "persona", Name: "conflict.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "conflict.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	// Verify it's a ConflictsError with correct data
	conflictsErr, ok := err.(*ConflictsError)
	if !ok {
		t.Fatalf("expected *ConflictsError, got %T", err)
	}
	if len(conflictsErr.Conflicts) != 1 {
		t.Errorf("len(Conflicts) = %d, want 1", len(conflictsErr.Conflicts))
	}
	if conflictsErr.Conflicts[0].Path != "output-styles/conflict.md" {
		t.Errorf("Conflict path = %q, want 'output-styles/conflict.md'", conflictsErr.Conflicts[0].Path)
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{Force: true})
	if err != nil {
		t.Fatalf("Install with force failed: %v", err)
	}
	if len(result.Added) != 1 {
		t.Error("file not added with force")
	}
}

func TestInstallerInstallMultipleConflicts(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Create two unmanaged files
	stylesDir := filepath.Join(env.TargetDir, "output-styles")
	if err := os.MkdirAll(stylesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stylesDir, "conflict1.md"), []byte("# Unmanaged 1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stylesDir, "conflict2.md"), []byte("# Unmanaged 2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create three assets - two with conflicts, one clean
	env.SetupAsset("personas", "conflict1.md", "---\ntai-schema-version: 1\nname: Conflict1\n---\n\n# New 1")
	env.SetupAsset("personas", "conflict2.md", "---\ntai-schema-version: 1\nname: Conflict2\n---\n\n# New 2")
	env.SetupAsset("personas", "clean.md", "---\ntai-schema-version: 1\nname: Clean\n---\n\n# Clean")

	assets := []*config.Asset{
		{Type: "persona", Name: "conflict1.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "conflict1.md")},
		{Type: "persona", Name: "conflict2.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "conflict2.md")},
		{Type: "persona", Name: "clean.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "clean.md")},
	}

	// Install should fail with all conflicts reported
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	conflictsErr, ok := err.(*ConflictsError)
	if !ok {
		t.Fatalf("expected *ConflictsError, got %T", err)
	}
	if len(conflictsErr.Conflicts) != 2 {
		t.Errorf("len(Conflicts) = %d, want 2", len(conflictsErr.Conflicts))
	}

	// Verify clean file was NOT written (no-op on conflict)
	if env.TargetFileExists("output-styles/clean.md") {
		t.Error("clean file should NOT be written when conflicts exist")
	}
}

func TestInstallerDryRunReportsConflicts(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Create unmanaged file
	targetPath := filepath.Join(env.TargetDir, "output-styles", "conflict.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("# Unmanaged"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupAsset("personas", "conflict.md", "---\ntai-schema-version: 1\nname: Conflict\n---\n\n# New Content")

	assets := []*config.Asset{
		{Type: "persona", Name: "conflict.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "conflict.md")},
	}

	// Dry-run should now report conflicts instead of silently proceeding
	// This is intentional: --dry-run --diff is a natural preview workflow
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{DryRun: true})
	if err == nil {
		t.Fatal("expected conflict error during dry-run, got nil")
	}

	conflictsErr, ok := err.(*ConflictsError)
	if !ok {
		t.Fatalf("expected *ConflictsError, got %T", err)
	}
	if len(conflictsErr.Conflicts) != 1 {
		t.Errorf("len(Conflicts) = %d, want 1", len(conflictsErr.Conflicts))
	}

	// With force flag, dry-run should proceed
	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{DryRun: true, Force: true})
	if err != nil {
		t.Fatalf("dry-run with force failed: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}
}

func TestInstallerInstallConflictSkip(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	targetPath := filepath.Join(env.TargetDir, "output-styles", "conflict.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("# Unmanaged"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupAsset("personas", "conflict.md", "---\ntai-schema-version: 1\nname: Conflict\n---\n\n# New Content")

	assets := []*config.Asset{
		{Type: "persona", Name: "conflict.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "conflict.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{SkipConflicts: true})
	if err != nil {
		t.Fatalf("Install with skip-conflicts failed: %v", err)
	}

	if len(result.Skipped) != 1 {
		t.Errorf("len(Skipped) = %d, want 1", len(result.Skipped))
	}

	content, _ := os.ReadFile(targetPath)
	if string(content) != "# Unmanaged" {
		t.Error("original content was overwritten")
	}
}

func TestInstallerUninstall(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test")

	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if !env.TargetFileExists("output-styles/test.md") {
		t.Fatal("file not installed")
	}

	result, err := installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	if len(result.Removed) != 1 {
		t.Errorf("len(Removed) = %d, want 1", len(result.Removed))
	}

	if env.TargetFileExists("output-styles/test.md") {
		t.Error("file not removed")
	}
}

func TestInstallerUninstallWithBackup(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	existingPath := filepath.Join(env.TargetDir, "CLAUDE.md")
	if err := os.WriteFile(existingPath, []byte("# Original"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupAsset("files", "CLAUDE.md", "# Wrapped\n{{existing}}")

	assets := []*config.Asset{
		{Type: "file", Name: "CLAUDE.md", SourceName: "local", Dest: "",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "CLAUDE.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	_, err = installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	content, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(content) != "# Original" {
		t.Errorf("restored content = %q, want '# Original'", string(content))
	}
}

func TestInstallerStatus(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	status, err := installer.Status(adapter.Name())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != nil {
		t.Error("expected nil status for no installation")
	}

	env.SetupAsset("personas", "test.md", "---\ntai-schema-version: 1\nname: Test\n---\n\n# Test")
	assets := []*config.Asset{
		{Type: "persona", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}
	_, err = installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	status, err = installer.Status(adapter.Name())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Profile != "test-profile" {
		t.Errorf("status.Profile = %q, want 'test-profile'", status.Profile)
	}
	if len(status.Files) != 1 {
		t.Errorf("len(status.Files) = %d, want 1", len(status.Files))
	}
}

func TestInstallerInstallSkillDirectory(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\ntai-schema-version: 1\nname: My Skill\ndescription: A helpful skill\n---\n\n# My Skill\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.sh"), []byte("#!/bin/bash\necho hello"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "skill", Name: "my-skill", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	if !env.TargetFileExists("skills/my-skill/SKILL.md") {
		t.Error("SKILL.md not created")
	}
	if !env.TargetFileExists("skills/my-skill/helper.sh") {
		t.Error("helper.sh not created")
	}

	// SKILL.md should be transformed with Claude agent frontmatter
	skillContent := env.ReadTargetFile("skills/my-skill/SKILL.md")
	expectedSkill := "---\nname: My Skill\ndescription: A helpful skill\n---\n\n# My Skill\n"
	if skillContent != expectedSkill {
		t.Errorf("SKILL.md content = %q, want %q", skillContent, expectedSkill)
	}
}

func TestCopyFilePreservesPermissions(t *testing.T) {
	dir := t.TempDir()

	srcPath := filepath.Join(dir, "script.sh")
	dstPath := filepath.Join(dir, "script-copy.sh")

	if err := os.WriteFile(srcPath, []byte("#!/bin/bash\necho hello"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}

	if dstInfo.Mode().Perm() != 0755 {
		t.Errorf("dst permissions = %o, want %o", dstInfo.Mode().Perm(), os.FileMode(0755))
	}
}

func TestInstallerPreservesFilePermissions(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Use a file asset with executable permissions to test permission preservation
	scriptPath := filepath.Join(env.ConfigRepo, "assets", "files", "run.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho run"), 0755); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "file", Name: "run.sh", SourceName: "local", Dest: "",
			SourcePath: scriptPath},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	installed := filepath.Join(env.TargetDir, "run.sh")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("stat installed file: %v", err)
	}

	if info.Mode().Perm() != 0755 {
		t.Errorf("installed permissions = %o, want %o", info.Mode().Perm(), os.FileMode(0755))
	}
}

func TestInstallerSkillDirectoryPreservesPermissions(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "mixed-perms")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "run.sh"), []byte("#!/bin/bash\necho run"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "README.md"), []byte("# Docs"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "skill", Name: "mixed-perms", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	scriptInfo, err := os.Stat(filepath.Join(env.TargetDir, "skills", "mixed-perms", "run.sh"))
	if err != nil {
		t.Fatalf("stat script: %v", err)
	}
	if scriptInfo.Mode().Perm() != 0755 {
		t.Errorf("script permissions = %o, want %o", scriptInfo.Mode().Perm(), os.FileMode(0755))
	}

	docInfo, err := os.Stat(filepath.Join(env.TargetDir, "skills", "mixed-perms", "README.md"))
	if err != nil {
		t.Fatalf("stat doc: %v", err)
	}
	if docInfo.Mode().Perm() != 0644 {
		t.Errorf("doc permissions = %o, want %o", docInfo.Mode().Perm(), os.FileMode(0644))
	}
}

func TestInstallAssetFallsBackTo0644WhenFileModeZero(t *testing.T) {
	dir := t.TempDir()
	installer := &Installer{}
	adapter := claude.New(dir)

	targetPath := filepath.Join(dir, "output.md")
	resolved := &config.ResolvedAsset{
		Asset:   config.Asset{Type: "file", Name: "output.md", Dest: ""},
		Content: []byte("# Hello"),
	}

	if err := installer.installAsset(resolved, targetPath, adapter); err != nil {
		t.Fatalf("installAsset failed: %v", err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("permissions = %o, want %o (default fallback)", info.Mode().Perm(), fs.FileMode(0644))
	}
}

func TestInstallerPermissionChangeTriggersUpdate(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	scriptPath := filepath.Join(env.ConfigRepo, "assets", "files", "run.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho v1"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "file", Name: "run.sh", SourceName: "local", Dest: "", SourcePath: scriptPath},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("first install: %v", err)
	}

	installed := filepath.Join(env.TargetDir, "run.sh")
	info, _ := os.Stat(installed)
	if info.Mode().Perm() != 0644 {
		t.Fatalf("first install permissions = %o, want 0644", info.Mode().Perm())
	}

	// Change source content and permissions
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho v2"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(scriptPath, 0755); err != nil {
		t.Fatal(err)
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if len(result.Updated) != 1 {
		t.Errorf("len(Updated) = %d, want 1", len(result.Updated))
	}

	info, _ = os.Stat(installed)
	if info.Mode().Perm() != 0755 {
		t.Errorf("updated permissions = %o, want 0755", info.Mode().Perm())
	}
}

func TestInstallerPersonaWithFrontmatter(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "senior-swe.md", "---\ntai-schema-version: 1\nname: Senior SWE\ndescription: An experienced engineer\n---\n\n# Senior SWE\n\nYou are a senior engineer.\n")

	assets := []*config.Asset{
		{Type: "persona", Name: "senior-swe.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "senior-swe.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("output-styles/senior-swe.md")
	expected := "---\nname: Senior SWE\ndescription: An experienced engineer\n---\n\n# Senior SWE\n\nYou are a senior engineer.\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerDelegateWithFrontmatter(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("delegates", "reviewer.md", "---\ntai-schema-version: 1\nname: Code Reviewer\ndescription: Reviews code\n---\n\nReview all code changes.\n")

	assets := []*config.Asset{
		{Type: "delegate", Name: "reviewer.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "reviewer.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("agents/reviewer.md")
	expected := "---\nname: Code Reviewer\ndescription: Reviews code\n---\n\nReview all code changes.\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerDelegateWithTools(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("delegates", "reviewer.md", "---\ntai-schema-version: 1\nname: Code Reviewer\ndescription: Reviews code\ntools: Read, Glob, Grep\n---\n\nReview all code changes.\n")

	assets := []*config.Asset{
		{Type: "delegate", Name: "reviewer.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "reviewer.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("agents/reviewer.md")
	expected := "---\nname: Code Reviewer\ndescription: Reviews code\ntools: Read, Glob, Grep\n---\n\nReview all code changes.\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerFilesCopiedAsIs(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// File assets are copied as-is without any frontmatter processing
	env.SetupAsset("files", "NOTES.md", "---\ntitle: Notes\n---\n\n# Project Notes\n\nSome reference material.\n")

	assets := []*config.Asset{
		{Type: "file", Name: "NOTES.md", SourceName: "local", Dest: "",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "NOTES.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// File content should be exactly as source (no frontmatter processing)
	content := env.ReadTargetFile("NOTES.md")
	expected := "---\ntitle: Notes\n---\n\n# Project Notes\n\nSome reference material.\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerFileWithCustomDest(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// File with custom destination path
	env.SetupAsset("files", "config.json", `{"key": "value"}`)

	assets := []*config.Asset{
		{Type: "file", Name: "config.json", SourceName: "local", Dest: "settings",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "config.json")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("settings/config.json")
	expected := `{"key": "value"}`
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerSkillDirectoryTransformsSKILLMd(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\ntai-schema-version: 1\nname: Test Skill\ndescription: Does testing\ntools: Read, Bash\n---\n\nUse this skill for testing.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "skill", Name: "test-skill", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// SKILL.md should get Claude agent frontmatter (name, description, tools)
	content := env.ReadTargetFile("skills/test-skill/SKILL.md")
	expected := "---\nname: Test Skill\ndescription: Does testing\ntools: Read, Bash\n---\n\nUse this skill for testing.\n"
	if content != expected {
		t.Errorf("SKILL.md content = %q, want %q", content, expected)
	}
}

// --- validateAssetMetadata unit tests ---

func TestValidateAssetMetadataValid(t *testing.T) {
	metadata := map[string]string{
		"tai-schema-version": "1",
		"name":               "My Asset",
	}

	for _, assetType := range []string{"persona", "delegate", "skill"} {
		if err := validateAssetMetadata(metadata, assetType); err != nil {
			t.Errorf("validateAssetMetadata(%s) unexpected error: %v", assetType, err)
		}
	}
}

func TestValidateAssetMetadataFileSkipsValidation(t *testing.T) {
	// File type assets skip frontmatter validation entirely
	metadata := map[string]string{
		"tai-schema-version": "1",
	}

	if err := validateAssetMetadata(metadata, "file"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateAssetMetadataNilMetadata(t *testing.T) {
	err := validateAssetMetadata(nil, "persona")
	if err == nil {
		t.Fatal("expected error for nil metadata")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want mention of 'name'", err.Error())
	}
}

func TestValidateAssetMetadataFileNilMetadataValid(t *testing.T) {
	// File assets don't require frontmatter
	if err := validateAssetMetadata(nil, "file"); err != nil {
		t.Errorf("file with nil metadata should be valid, got: %v", err)
	}
}

func TestValidateAssetMetadataSchemaVersionOptional(t *testing.T) {
	// Schema version is recognized-but-optional; assets without it are valid
	metadata := map[string]string{
		"name": "Test",
	}

	for _, assetType := range []string{"persona", "delegate", "skill"} {
		if err := validateAssetMetadata(metadata, assetType); err != nil {
			t.Errorf("validateAssetMetadata(%s) without schema version: unexpected error: %v", assetType, err)
		}
	}
}

func TestValidateAssetMetadataIgnoresTaiSchemaVersion(t *testing.T) {
	// tai-schema-version is treated as any other unknown field — silently ignored
	metadata := map[string]string{
		"tai-schema-version": "99",
		"name":               "Test",
	}

	if err := validateAssetMetadata(metadata, "persona"); err != nil {
		t.Errorf("unexpected error (tai-schema-version should be ignored): %v", err)
	}
}

func TestValidateAssetMetadataMissingName(t *testing.T) {
	metadata := map[string]string{
		"tai-schema-version": "1",
	}

	for _, assetType := range []string{"persona", "delegate", "skill"} {
		err := validateAssetMetadata(metadata, assetType)
		if err == nil {
			t.Errorf("validateAssetMetadata(%s) expected error for missing name", assetType)
			continue
		}
		if !strings.Contains(err.Error(), "name") {
			t.Errorf("error = %q, want mention of 'name'", err.Error())
		}
	}
}

func TestValidateAssetMetadataWhitespaceOnlyName(t *testing.T) {
	metadata := map[string]string{
		"tai-schema-version": "1",
		"name":               "   \t  ",
	}

	err := validateAssetMetadata(metadata, "persona")
	if err == nil {
		t.Fatal("expected error for whitespace-only name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want mention of 'name'", err.Error())
	}
}

func TestValidateAssetMetadataUnknownAssetType(t *testing.T) {
	metadata := map[string]string{
		"tai-schema-version": "1",
		"name":               "Test",
	}

	err := validateAssetMetadata(metadata, "unknown-type")
	if err == nil {
		t.Fatal("expected error for unknown asset type")
	}
	if !strings.Contains(err.Error(), "unknown asset type") {
		t.Errorf("error = %q, want mention of 'unknown asset type'", err.Error())
	}
}

func TestValidateAssetMetadataIgnoresUnknownFields(t *testing.T) {
	tests := []struct {
		assetType string
		field     string
	}{
		{"persona", "tools"},     // tools not recognized on personas
		{"file", "tools"},        // tools not recognized on files
		{"persona", "author"},    // completely unknown field
		{"delegate", "priority"}, // completely unknown field
		{"skill", "version"},     // not a tai frontmatter field
	}

	for _, tt := range tests {
		metadata := map[string]string{
			"name":   "Test",
			tt.field: "some value",
		}

		if err := validateAssetMetadata(metadata, tt.assetType); err != nil {
			t.Errorf("validateAssetMetadata(%s) with field %q: unexpected error: %v", tt.assetType, tt.field, err)
		}
	}
}

func TestValidateAssetMetadataAllowsValidFields(t *testing.T) {
	// Delegates and skills allow tools
	for _, assetType := range []string{"delegate", "skill"} {
		metadata := map[string]string{
			"tai-schema-version": "1",
			"name":               "Test",
			"description":        "A description",
			"tools":              "Read, Bash",
		}
		if err := validateAssetMetadata(metadata, assetType); err != nil {
			t.Errorf("validateAssetMetadata(%s) with all valid fields: unexpected error: %v", assetType, err)
		}
	}

	// Personas allow name + description (no tools)
	metadata := map[string]string{
		"tai-schema-version": "1",
		"name":               "Test",
		"description":        "A description",
	}
	if err := validateAssetMetadata(metadata, "persona"); err != nil {
		t.Errorf("validateAssetMetadata(persona) with valid fields: unexpected error: %v", err)
	}
}

// --- Integration validation tests ---

func TestInstallerRejectsMissingName(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Asset without any frontmatter — no name field
	env.SetupAsset("personas", "bad.md", "# No Frontmatter")

	assets := []*config.Asset{
		{Type: "persona", Name: "bad.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "bad.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want mention of 'name'", err.Error())
	}
}

func TestInstallerIgnoresTaiSchemaVersion(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// tai-schema-version (any value) is treated as an unknown field — ignored
	env.SetupAsset("personas", "versioned.md", "---\ntai-schema-version: 99\nname: Versioned\n---\n\n# Content")

	assets := []*config.Asset{
		{Type: "persona", Name: "versioned.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "versioned.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install should succeed (tai-schema-version is ignored), got: %v", err)
	}

	// Verify tai-schema-version is stripped from output (adapter doesn't render it)
	content := env.ReadTargetFile("output-styles/versioned.md")
	if strings.Contains(content, "tai-schema-version") {
		t.Error("installed file should not contain tai-schema-version")
	}
}

func TestInstallerRejectsMissingNameForPersona(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// tai-schema-version present but no name
	env.SetupAsset("personas", "bad.md", "---\ntai-schema-version: 1\n---\n\n# No Name")

	assets := []*config.Asset{
		{Type: "persona", Name: "bad.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "bad.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want mention of 'name'", err.Error())
	}
}

func TestInstallerFileSkipsValidation(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Files skip frontmatter validation — content is copied as-is
	env.SetupAsset("files", "notes.md", "---\ntai-schema-version: 1\n---\n\n# Notes")

	assets := []*config.Asset{
		{Type: "file", Name: "notes.md", SourceName: "local", Dest: "",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "notes.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install should succeed for file, got: %v", err)
	}

	// File content is preserved exactly (no frontmatter stripping)
	content := env.ReadTargetFile("notes.md")
	expected := "---\ntai-schema-version: 1\n---\n\n# Notes"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

// --- Regression tests ---

func TestInstallerOutputNeverContainsTaiSchemaVersion(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Install transformed asset types and verify tai-schema-version is stripped from output
	// (Note: file type is excluded because it preserves content as-is without transformation)
	env.SetupAsset("personas", "p.md", "---\ntai-schema-version: 1\nname: P\ndescription: A persona\n---\n\n# Persona")
	env.SetupAsset("delegates", "d.md", "---\ntai-schema-version: 1\nname: D\ntools: Read, Bash\n---\n\n# Delegate")

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "s")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\ntai-schema-version: 1\nname: S\ntools: Grep\n---\n\n# Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "persona", Name: "p.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "p.md")},
		{Type: "delegate", Name: "d.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "d.md")},
		{Type: "skill", Name: "s", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify no installed file contains tai-schema-version
	filesToCheck := []string{
		"output-styles/p.md",
		"agents/d.md",
		"skills/s/SKILL.md",
	}
	for _, f := range filesToCheck {
		content := env.ReadTargetFile(f)
		if strings.Contains(content, "tai-schema-version") {
			t.Errorf("installed file %s contains tai-schema-version — should have been stripped", f)
		}
	}
}

func TestInstallerSkillDirectoryRejectsInvalidSKILLMd(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Skill directory with a SKILL.md that has no frontmatter (no name)
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "bad-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("# No frontmatter at all"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "skill", Name: "bad-skill", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("expected error for SKILL.md without name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want mention of 'name'", err.Error())
	}
}

func TestInstallerSkillNoSchemaVersion(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Skill with name but no tai-schema-version — should succeed
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "no-version")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: No Version Skill\ndescription: Works without schema version\n---\n\n# No Version\n"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "skill", Name: "no-version", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install should succeed without tai-schema-version, got: %v", err)
	}

	content := env.ReadTargetFile("skills/no-version/SKILL.md")
	expected := "---\nname: No Version Skill\ndescription: Works without schema version\n---\n\n# No Version\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerPersonaNoSchemaVersion(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Persona with name but no tai-schema-version — should succeed
	env.SetupAsset("personas", "minimal.md", "---\nname: Minimal\n---\n\n# Minimal Persona")

	assets := []*config.Asset{
		{Type: "persona", Name: "minimal.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "minimal.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install should succeed without tai-schema-version, got: %v", err)
	}
	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	content := env.ReadTargetFile("output-styles/minimal.md")
	expected := "---\nname: Minimal\n---\n\n# Minimal Persona"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerFileNoFrontmatter(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// File with no frontmatter — content is copied exactly as-is
	env.SetupAsset("files", "plain.md", "# Just a file\n\nNo frontmatter here.\n")

	assets := []*config.Asset{
		{Type: "file", Name: "plain.md", SourceName: "local", Dest: "",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "plain.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install should succeed for file, got: %v", err)
	}

	content := env.ReadTargetFile("plain.md")
	expected := "# Just a file\n\nNo frontmatter here.\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerSkillWithFieldMapping(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Create an external source with a SKILL.md using non-tai field names
	externalSource := filepath.Join(env.Root, "external")
	if err := os.MkdirAll(externalSource, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalSource, "SKILL.md"),
		[]byte("---\ntitle: External Skill\nsummary: An externally-sourced skill\n---\n\n# External Skill Content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create skill directory with tai.yaml that includes the external SKILL.md with field mapping
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "mapped-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifestContent := `include:
  - from: external
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title
      description: summary
`
	if err := os.WriteFile(filepath.Join(skillDir, "tai.yaml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	sources := map[string]*source.LoadedSource{
		"external": {
			Source:       config.Source{Name: "external", Path: externalSource},
			ResolvedPath: externalSource,
		},
	}

	manifestStore := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := NewInstaller(manifestStore, env.ConfigRepo, sources)
	adapter := claude.New(env.TargetDir)

	assets := []*config.Asset{
		{Type: "skill", Name: "mapped-skill", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install with field mapping failed: %v", err)
	}

	content := env.ReadTargetFile("skills/mapped-skill/SKILL.md")
	expected := "---\nname: External Skill\ndescription: An externally-sourced skill\n---\n\n# External Skill Content\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerSkillFieldMappingLocalValueWins(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Source file has both "title" and "name" — local "name" should win
	externalSource := filepath.Join(env.Root, "external")
	if err := os.MkdirAll(externalSource, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(externalSource, "SKILL.md"),
		[]byte("---\nname: Local Name\ntitle: Mapped Title\n---\n\n# Content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "precedence")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifestContent := `include:
  - from: external
    path: SKILL.md
    dest: SKILL.md
    fields:
      name: title
`
	if err := os.WriteFile(filepath.Join(skillDir, "tai.yaml"), []byte(manifestContent), 0644); err != nil {
		t.Fatal(err)
	}

	sources := map[string]*source.LoadedSource{
		"external": {
			Source:       config.Source{Name: "external", Path: externalSource},
			ResolvedPath: externalSource,
		},
	}

	manifestStore := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := NewInstaller(manifestStore, env.ConfigRepo, sources)
	adapter := claude.New(env.TargetDir)

	assets := []*config.Asset{
		{Type: "skill", Name: "precedence", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// "name" was already set, so mapping from "title" should not override it.
	// "title" is deleted since it was the mapping source.
	content := env.ReadTargetFile("skills/precedence/SKILL.md")
	expected := "---\nname: Local Name\n---\n\n# Content\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

// --- findSkillMdFields and isRootSkillMdDest tests ---

func TestIsRootSkillMdDest(t *testing.T) {
	tests := []struct {
		name string
		inc  config.Include
		want bool
	}{
		{
			name: "explicit dest SKILL.md",
			inc:  config.Include{From: "src", Path: "README.md", Dest: "SKILL.md"},
			want: true,
		},
		{
			name: "trailing slash root with path to SKILL.md",
			inc:  config.Include{From: "src", Path: "docs/SKILL.md", Dest: "./"},
			want: true,
		},
		{
			name: "bare trailing slash with path to SKILL.md",
			inc:  config.Include{From: "src", Path: "SKILL.md", Dest: "/"},
			want: true,
		},
		{
			name: "trailing slash with non-SKILL.md path",
			inc:  config.Include{From: "src", Path: "docs/README.md", Dest: "./"},
			want: false,
		},
		{
			name: "subdirectory trailing slash with SKILL.md path",
			inc:  config.Include{From: "src", Path: "SKILL.md", Dest: "subdir/"},
			want: false,
		},
		{
			name: "explicit dest in subdirectory",
			inc:  config.Include{From: "src", Path: "SKILL.md", Dest: "sub/SKILL.md"},
			want: false,
		},
		{
			name: "URL-based trailing slash root with SKILL.md",
			inc:  config.Include{URL: "https://example.com/path/SKILL.md", Dest: "./"},
			want: true,
		},
		{
			name: "URL-based trailing slash with other file",
			inc:  config.Include{URL: "https://example.com/path/other.md", Dest: "./"},
			want: false,
		},
		{
			name: "URL-based subdirectory trailing slash with SKILL.md",
			inc:  config.Include{URL: "https://example.com/SKILL.md", Dest: "sub/"},
			want: false,
		},
		{
			name: "URL with query params",
			inc:  config.Include{URL: "https://example.com/SKILL.md?raw=true", Dest: "./"},
			want: true,
		},
		{
			name: "unrelated include",
			inc:  config.Include{From: "src", Path: "SYNTAX.md", Dest: "references/"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRootSkillMdDest(tt.inc)
			if got != tt.want {
				t.Errorf("isRootSkillMdDest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindSkillMdFields(t *testing.T) {
	t.Run("nil manifest", func(t *testing.T) {
		result := findSkillMdFields(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("no matching include", func(t *testing.T) {
		manifest := &config.SkillManifest{
			Include: []config.Include{
				{From: "src", Path: "SYNTAX.md", Dest: "references/"},
			},
		}
		result := findSkillMdFields(manifest)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("matching include with fields", func(t *testing.T) {
		manifest := &config.SkillManifest{
			Include: []config.Include{
				{From: "src", Path: "SYNTAX.md", Dest: "references/"},
				{From: "ext", Path: "SKILL.md", Dest: "SKILL.md", Fields: map[string]string{
					"name": "title",
				}},
			},
		}
		result := findSkillMdFields(manifest)
		if result == nil {
			t.Fatal("expected non-nil fields")
		}
		if result["name"] != "title" {
			t.Errorf("fields[name] = %q, want 'title'", result["name"])
		}
	})

	t.Run("matching include without fields", func(t *testing.T) {
		manifest := &config.SkillManifest{
			Include: []config.Include{
				{From: "ext", Path: "SKILL.md", Dest: "SKILL.md"},
			},
		}
		result := findSkillMdFields(manifest)
		if result != nil {
			t.Errorf("expected nil for include without fields, got %v", result)
		}
	})
}

// --- Templated dest path tests ---

func TestInstallerFileWithTemplatedDest(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// File with templated dest using adapter-provided variable
	env.SetupAsset("files", "AGENTS.md", "# Global Instructions\n\nBe helpful.")

	assets := []*config.Asset{
		{Type: "file", Name: "AGENTS.md", SourceName: "local",
			Dest:       "{{target_agents_file}}",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "AGENTS.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	// Dest should resolve to CLAUDE.md (from adapter's target_agents_file)
	if !env.TargetFileExists("CLAUDE.md") {
		t.Error("CLAUDE.md not created (expected from {{target_agents_file}})")
	}

	content := env.ReadTargetFile("CLAUDE.md")
	expected := "# Global Instructions\n\nBe helpful."
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerTemplatedDestWithUserOverride(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// File with templated dest - user variable should override adapter
	env.SetupAsset("files", "AGENTS.md", "# Global Instructions")

	assets := []*config.Asset{
		{Type: "file", Name: "AGENTS.md", SourceName: "local",
			Dest:       "{{target_agents_file}}",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "AGENTS.md")},
	}

	// User provides override via --var
	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{
		Variables: template.Variables{"target_agents_file": "CUSTOM.md"},
	})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	// User override should win
	if !env.TargetFileExists("CUSTOM.md") {
		t.Error("CUSTOM.md not created (expected from user override)")
	}
	if env.TargetFileExists("CLAUDE.md") {
		t.Error("CLAUDE.md should not exist (user override should take precedence)")
	}
}

func TestInstallerTemplatedDestUnknownVar(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// File with unknown template variable - pongo2 renders it as empty string,
	// which produces an empty dest path and should fail validation.
	env.SetupAsset("files", "test.md", "# Test")

	assets := []*config.Asset{
		{Type: "file", Name: "test.md", SourceName: "local",
			Dest:       "{{unknown_var}}",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "test.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("expected error for unknown variable in dest path, got nil")
	}
	if !strings.Contains(err.Error(), "resolved to empty string") {
		t.Errorf("error = %q, expected to mention 'resolved to empty string'", err.Error())
	}
}

func TestInstallerTemplatedDestWithSubdir(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Template variable in middle of dest path
	env.SetupAsset("files", "config.json", `{"key": "value"}`)

	assets := []*config.Asset{
		{Type: "file", Name: "config.json", SourceName: "local",
			Dest:       "{{target_name}}-config",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "config.json")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	// Should resolve to claude-config/config.json
	if !env.TargetFileExists("claude-config/config.json") {
		t.Error("claude-config/config.json not created")
	}
}

func TestInstallerTemplatedDestPathTraversalRejected(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("files", "malicious.md", "# Malicious")

	assets := []*config.Asset{
		{Type: "file", Name: "malicious.md", SourceName: "local",
			Dest:       "{{evil_path}}",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "malicious.md")},
	}

	// Attempt path traversal via template variable
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{
		Variables: template.Variables{"evil_path": "../../../tmp/pwned"},
	})
	if err == nil {
		t.Fatal("Install should have failed for path traversal attempt")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("error should mention path traversal, got: %v", err)
	}
}

func TestInstallerTemplatedDestAbsolutePathRejected(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("files", "config.md", "# Config")

	assets := []*config.Asset{
		{Type: "file", Name: "config.md", SourceName: "local",
			Dest:       "{{abs_path}}",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "config.md")},
	}

	// Attempt absolute path injection via template variable
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{
		Variables: template.Variables{"abs_path": "/etc/passwd"},
	})
	if err == nil {
		t.Fatal("Install should have failed for absolute path injection")
	}
	if !strings.Contains(err.Error(), "absolute paths not allowed") {
		t.Errorf("error should mention absolute paths, got: %v", err)
	}
}

func TestInstallerTemplatedDestExistingVarRejected(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("files", "config.md", "# Config")

	assets := []*config.Asset{
		{Type: "file", Name: "config.md", SourceName: "local",
			Dest:       "{{existing}}/subdir",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "files", "config.md")},
	}

	// {{existing}} doesn't make sense in dest paths
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("Install should have failed for {{existing}} in dest path")
	}
	if !strings.Contains(err.Error(), "{{existing}} cannot be used in dest path") {
		t.Errorf("error should explain {{existing}} limitation, got: %v", err)
	}
}
