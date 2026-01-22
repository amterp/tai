package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
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

	// Create test assets
	env.SetupAsset("personas", "test.md", "# Test Persona")
	env.SetupAsset("delegates", "helper.md", "# Helper Agent")

	assets := []*config.Asset{
		{Type: "personas", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
		{Type: "delegates", Name: "helper.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "helper.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Check results
	if len(result.Added) != 2 {
		t.Errorf("len(Added) = %d, want 2", len(result.Added))
	}
	if len(result.Updated) != 0 {
		t.Errorf("len(Updated) = %d, want 0", len(result.Updated))
	}

	// Verify files were created
	if !env.TargetFileExists("output-styles/test.md") {
		t.Error("output-styles/test.md not created")
	}
	if !env.TargetFileExists("agents/helper.md") {
		t.Error("agents/helper.md not created")
	}

	// Verify content
	content := env.ReadTargetFile("output-styles/test.md")
	if content != "# Test Persona" {
		t.Errorf("content = %q, want '# Test Persona'", content)
	}

	// Verify manifest was saved
	if result.Manifest == nil {
		t.Fatal("Manifest is nil")
	}
	if result.Manifest.Profile != "test-profile" {
		t.Errorf("Manifest.Profile = %q, want 'test-profile'", result.Manifest.Profile)
	}
}

func TestInstallerInstallDryRun(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "test.md", "# Test Persona")

	assets := []*config.Asset{
		{Type: "personas", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Should report as added
	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	// But file should NOT exist
	if env.TargetFileExists("output-styles/test.md") {
		t.Error("file created during dry run")
	}
}

func TestInstallerInstallUpdate(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Initial install
	env.SetupAsset("personas", "test.md", "# Version 1")
	assets := []*config.Asset{
		{Type: "personas", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Update the asset
	env.SetupAsset("personas", "test.md", "# Version 2")

	// Re-install
	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	// Should be an update
	if len(result.Updated) != 1 {
		t.Errorf("len(Updated) = %d, want 1", len(result.Updated))
	}

	// Verify new content
	content := env.ReadTargetFile("output-styles/test.md")
	if content != "# Version 2" {
		t.Errorf("content = %q, want '# Version 2'", content)
	}
}

func TestInstallerInstallRemoveStale(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Initial install with two assets
	env.SetupAsset("personas", "keep.md", "# Keep")
	env.SetupAsset("personas", "remove.md", "# Remove")

	assets := []*config.Asset{
		{Type: "personas", Name: "keep.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "keep.md")},
		{Type: "personas", Name: "remove.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "remove.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Re-install with only one asset
	assets = []*config.Asset{
		{Type: "personas", Name: "keep.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "keep.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}

	// Should report removed
	if len(result.Removed) != 1 {
		t.Errorf("len(Removed) = %d, want 1", len(result.Removed))
	}

	// Verify file was removed
	if env.TargetFileExists("output-styles/remove.md") {
		t.Error("removed file still exists")
	}
	if !env.TargetFileExists("output-styles/keep.md") {
		t.Error("kept file was removed")
	}
}

func TestInstallerInstallWithTemplating(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	env.SetupAsset("personas", "templated.md", "# Persona for {{project_name}}")

	assets := []*config.Asset{
		{Type: "personas", Name: "templated.md", SourceName: "local",
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

	// Verify templated content
	content := env.ReadTargetFile("output-styles/templated.md")
	expected := "# Persona for MyProject"
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

	// Create asset with {{existing}}
	env.SetupAsset("docs", "CLAUDE.md", "# Header\n\n{{existing}}\n\n# Footer")

	assets := []*config.Asset{
		{Type: "docs", Name: "CLAUDE.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "CLAUDE.md")},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify backup was created
	if len(result.Manifest.Files) != 1 {
		t.Fatal("expected 1 manifest entry")
	}
	if result.Manifest.Files[0].BackupPath == "" {
		t.Error("BackupPath is empty")
	}

	// Verify templated content includes existing
	content := env.ReadTargetFile("CLAUDE.md")
	expected := "# Header\n\n# Existing Content\n\n# Footer"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}

	// Verify backup exists
	backupPath := existingPath + ".tai-backup"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file not created")
	}
}

func TestInstallerInstallConflictUnmanaged(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Create existing unmanaged file at target
	targetPath := filepath.Join(env.TargetDir, "output-styles", "conflict.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("# Unmanaged"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupAsset("personas", "conflict.md", "# New Content")

	assets := []*config.Asset{
		{Type: "personas", Name: "conflict.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "conflict.md")},
	}

	// Should fail without --force
	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	// Should succeed with --force
	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{Force: true})
	if err != nil {
		t.Fatalf("Install with force failed: %v", err)
	}
	if len(result.Added) != 1 {
		t.Error("file not added with force")
	}
}

func TestInstallerInstallConflictSkip(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Create existing unmanaged file
	targetPath := filepath.Join(env.TargetDir, "output-styles", "conflict.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("# Unmanaged"), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupAsset("personas", "conflict.md", "# New Content")

	assets := []*config.Asset{
		{Type: "personas", Name: "conflict.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "conflict.md")},
	}

	// Should succeed with --skip-conflicts
	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{SkipConflicts: true})
	if err != nil {
		t.Fatalf("Install with skip-conflicts failed: %v", err)
	}

	if len(result.Skipped) != 1 {
		t.Errorf("len(Skipped) = %d, want 1", len(result.Skipped))
	}

	// Original content should be preserved
	content, _ := os.ReadFile(targetPath)
	if string(content) != "# Unmanaged" {
		t.Error("original content was overwritten")
	}
}

func TestInstallerUninstall(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// First install
	env.SetupAsset("personas", "test.md", "# Test")

	assets := []*config.Asset{
		{Type: "personas", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify installed
	if !env.TargetFileExists("output-styles/test.md") {
		t.Fatal("file not installed")
	}

	// Uninstall
	result, err := installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	if len(result.Removed) != 1 {
		t.Errorf("len(Removed) = %d, want 1", len(result.Removed))
	}

	// Verify removed
	if env.TargetFileExists("output-styles/test.md") {
		t.Error("file not removed")
	}
}

func TestInstallerUninstallWithBackup(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Create existing file at target
	existingPath := filepath.Join(env.TargetDir, "CLAUDE.md")
	if err := os.WriteFile(existingPath, []byte("# Original"), 0644); err != nil {
		t.Fatal(err)
	}

	// Install with {{existing}}
	env.SetupAsset("docs", "CLAUDE.md", "# Wrapped\n{{existing}}")

	assets := []*config.Asset{
		{Type: "docs", Name: "CLAUDE.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "CLAUDE.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Uninstall
	_, err = installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Original content should be restored
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

	// No installation yet
	status, err := installer.Status(adapter.Name())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status != nil {
		t.Error("expected nil status for no installation")
	}

	// Install
	env.SetupAsset("personas", "test.md", "# Test")
	assets := []*config.Asset{
		{Type: "personas", Name: "test.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")},
	}
	_, err = installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Check status
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

	// Create a skill directory
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "helper.sh"), []byte("#!/bin/bash\necho hello"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "skills", Name: "my-skill", SourceName: "local",
			SourcePath: skillDir, IsDir: true},
	}

	result, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if len(result.Added) != 1 {
		t.Errorf("len(Added) = %d, want 1", len(result.Added))
	}

	// Verify directory was copied
	if !env.TargetFileExists("skills/my-skill/SKILL.md") {
		t.Error("SKILL.md not created")
	}
	if !env.TargetFileExists("skills/my-skill/helper.sh") {
		t.Error("helper.sh not created")
	}
}
