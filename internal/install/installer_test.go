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

	env.SetupAsset("docs", "CLAUDE.md", "---\ntai-schema-version: 1\n---\n\n# Header\n\n{{existing}}\n\n# Footer")

	assets := []*config.Asset{
		{Type: "doc", Name: "CLAUDE.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "CLAUDE.md")},
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

	// Docs produce body only (no target frontmatter)
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

	env.SetupAsset("docs", "CLAUDE.md", "---\ntai-schema-version: 1\n---\n\n# Wrapped\n{{existing}}")

	assets := []*config.Asset{
		{Type: "doc", Name: "CLAUDE.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "CLAUDE.md")},
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

	// Use a doc asset with executable permissions to test permission preservation
	scriptPath := filepath.Join(env.ConfigRepo, "assets", "docs", "run.sh")
	if err := os.WriteFile(scriptPath, []byte("---\ntai-schema-version: 1\n---\n\n#!/bin/bash\necho run"), 0755); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "doc", Name: "run.sh", SourceName: "local",
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
		Asset:   config.Asset{Type: "doc", Name: "output.md"},
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

	scriptPath := filepath.Join(env.ConfigRepo, "assets", "docs", "run.sh")
	if err := os.WriteFile(scriptPath, []byte("---\ntai-schema-version: 1\n---\n\n#!/bin/bash\necho v1"), 0644); err != nil {
		t.Fatal(err)
	}

	assets := []*config.Asset{
		{Type: "doc", Name: "run.sh", SourceName: "local", SourcePath: scriptPath},
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
	if err := os.WriteFile(scriptPath, []byte("---\ntai-schema-version: 1\n---\n\n#!/bin/bash\necho v2"), 0755); err != nil {
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

func TestInstallerDocStripsOnlyTaiFrontmatter(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Doc with tai frontmatter — installed content should be body only
	env.SetupAsset("docs", "NOTES.md", "---\ntai-schema-version: 1\nname: Notes\n---\n\n# Project Notes\n\nSome reference material.\n")

	assets := []*config.Asset{
		{Type: "doc", Name: "NOTES.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "NOTES.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("NOTES.md")
	expected := "# Project Notes\n\nSome reference material.\n"
	if content != expected {
		t.Errorf("content = %q, want %q", content, expected)
	}
}

func TestInstallerDocPreservesBodyFrontmatter(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Doc whose body itself contains --- blocks (e.g., wrapping a file with its own frontmatter).
	// Only the first (tai) frontmatter is stripped; the body's --- blocks are preserved.
	env.SetupAsset("docs", "REF.md", "---\ntai-schema-version: 1\n---\n\n---\ntitle: Reference\n---\n\n# Reference Doc\n")

	assets := []*config.Asset{
		{Type: "doc", Name: "REF.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "REF.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content := env.ReadTargetFile("REF.md")
	expected := "---\ntitle: Reference\n---\n\n# Reference Doc\n"
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

func TestValidateAssetMetadataDocAllowsMissingName(t *testing.T) {
	metadata := map[string]string{
		"tai-schema-version": "1",
	}

	if err := validateAssetMetadata(metadata, "doc"); err != nil {
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

func TestValidateAssetMetadataDocNilMetadataValid(t *testing.T) {
	if err := validateAssetMetadata(nil, "doc"); err != nil {
		t.Errorf("doc with nil metadata should be valid, got: %v", err)
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
		{"doc", "tools"},         // tools not recognized on docs
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

func TestInstallerDocAllowsMissingName(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Docs have no required fields — name is optional
	env.SetupAsset("docs", "notes.md", "---\ntai-schema-version: 1\n---\n\n# Notes")

	assets := []*config.Asset{
		{Type: "doc", Name: "notes.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "notes.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install should succeed for doc without name, got: %v", err)
	}

	content := env.ReadTargetFile("notes.md")
	if content != "# Notes" {
		t.Errorf("content = %q, want %q", content, "# Notes")
	}
}

// --- Regression tests ---

func TestInstallerOutputNeverContainsTaiSchemaVersion(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Install all asset types and verify tai-schema-version is stripped from output
	env.SetupAsset("personas", "p.md", "---\ntai-schema-version: 1\nname: P\ndescription: A persona\n---\n\n# Persona")
	env.SetupAsset("delegates", "d.md", "---\ntai-schema-version: 1\nname: D\ntools: Read, Bash\n---\n\n# Delegate")
	env.SetupAsset("docs", "doc.md", "---\ntai-schema-version: 1\nname: Doc\n---\n\n# Documentation")

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
		{Type: "doc", Name: "doc.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "doc.md")},
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
		"doc.md",
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

func TestInstallerDocNoFrontmatter(t *testing.T) {
	env, installer, adapter := setupInstallTest(t)

	// Doc with no frontmatter at all — should succeed, body is entire file
	env.SetupAsset("docs", "plain.md", "# Just a doc\n\nNo frontmatter here.\n")

	assets := []*config.Asset{
		{Type: "doc", Name: "plain.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "docs", "plain.md")},
	}

	_, err := installer.Install(adapter, "test-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Install should succeed for doc without frontmatter, got: %v", err)
	}

	content := env.ReadTargetFile("plain.md")
	expected := "# Just a doc\n\nNo frontmatter here.\n"
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
