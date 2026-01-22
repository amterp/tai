package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/testutil"
)

// TestE2EUninstall tests basic uninstall functionality.
func TestE2EUninstall(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("personas", "test.md", "# Test")
	env.SetupAsset("delegates", "helper.md", "# Helper")
	env.SetupBundle("test", `name: test
assets:
  - personas/test.md
  - delegates/helper.md
`)
	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	// Install first
	asset1, _ := sources["local"].ResolveAsset("personas/test.md")
	asset2, _ := sources["local"].ResolveAsset("delegates/helper.md")
	_, err := installer.Install(adapter, "test", []*config.Asset{asset1, asset2}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify files exist
	if !env.TargetFileExists("output-styles/test.md") {
		t.Fatal("test.md not installed")
	}
	if !env.TargetFileExists("agents/helper.md") {
		t.Fatal("helper.md not installed")
	}

	// Uninstall
	result, err := installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Verify results
	if len(result.Removed) != 2 {
		t.Errorf("Removed = %d, want 2", len(result.Removed))
	}

	// Verify files removed
	if env.TargetFileExists("output-styles/test.md") {
		t.Error("test.md still exists after uninstall")
	}
	if env.TargetFileExists("agents/helper.md") {
		t.Error("helper.md still exists after uninstall")
	}

	// Verify manifest deleted
	manifest, _ := installer.Status(adapter.Name())
	if manifest != nil {
		t.Error("Manifest still exists after uninstall")
	}
}

// TestE2EUninstallWithBackup tests that backups are restored on uninstall.
func TestE2EUninstallWithBackup(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Create existing file in target
	if err := os.MkdirAll(env.TargetDir, 0755); err != nil {
		t.Fatal(err)
	}
	originalContent := "# Original User Content"
	if err := os.WriteFile(filepath.Join(env.TargetDir, "CLAUDE.md"), []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("docs", "CLAUDE.md", "# Wrapped\n{{existing}}\n# End")
	env.SetupBundle("test", `name: test
assets:
  - docs/CLAUDE.md
`)
	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	// Install (wraps existing content)
	asset, _ := sources["local"].ResolveAsset("docs/CLAUDE.md")
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify wrapped content
	wrappedContent := env.ReadTargetFile("CLAUDE.md")
	if wrappedContent == originalContent {
		t.Error("Content was not wrapped")
	}

	// Uninstall
	_, err = installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Verify original content restored
	restoredContent := env.ReadTargetFile("CLAUDE.md")
	if restoredContent != originalContent {
		t.Errorf("Restored content = %q, want %q", restoredContent, originalContent)
	}

	// Verify backup removed
	backupPath := filepath.Join(env.TargetDir, "CLAUDE.md.tai-backup")
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("Backup file still exists")
	}
}

// TestE2EUninstallPreservesUnmanaged tests that unmanaged files are preserved.
func TestE2EUninstallPreservesUnmanaged(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
`)

	env.SetupAsset("personas", "managed.md", "# Managed")
	env.SetupBundle("test", `name: test
assets:
  - personas/managed.md
`)
	env.SetupProfile("test", `name: test
target: claude
bundles:
  - test
`)

	loader := source.NewLoader(env.ConfigRepo)
	taiConfig, _ := config.ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
	sources, _ := loader.LoadAll(taiConfig.Sources)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	// Install
	asset, _ := sources["local"].ResolveAsset("personas/managed.md")
	_, err := installer.Install(adapter, "test", []*config.Asset{asset}, install.InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Create an unmanaged file in the target
	unmanagedPath := filepath.Join(env.TargetDir, "output-styles", "unmanaged.md")
	if err := os.WriteFile(unmanagedPath, []byte("# Unmanaged"), 0644); err != nil {
		t.Fatal(err)
	}

	// Uninstall
	_, err = installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Verify managed file removed
	if env.TargetFileExists("output-styles/managed.md") {
		t.Error("managed.md still exists")
	}

	// Verify unmanaged file preserved
	if _, err := os.Stat(unmanagedPath); os.IsNotExist(err) {
		t.Error("unmanaged.md was removed - should have been preserved")
	}
}

// TestE2EUninstallEmpty tests uninstalling when nothing is installed.
func TestE2EUninstallEmpty(t *testing.T) {
	env := testutil.NewTestEnv(t)

	manifestStore := install.NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, env.ConfigRepo)
	adapter := claude.New(env.TargetDir)

	result, err := installer.Uninstall(adapter)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	if len(result.Removed) != 0 {
		t.Errorf("Removed = %d, want 0", len(result.Removed))
	}
}
