package install

import (
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/adapter/opencode"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func setupOpencodeInstallTest(t *testing.T) (*testutil.TestEnv, *Installer, *opencode.Adapter) {
	t.Helper()
	env := testutil.NewTestEnv(t)
	manifestStore := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	installer := NewInstaller(manifestStore, env.ConfigRepo, nil)
	adapter := opencode.New(env.TargetDir)
	return env, installer, adapter
}

// TestInstallerOpencodePersonaDelegateCollisionIsIdempotent guards against a
// reinstall reporting a phantom "updated" when a persona and a delegate share a
// name. OpenCode maps both to agents/<name>.md, so they collide on one target
// path. The collision must collapse to a single manifest entry (later wins),
// otherwise the shadowed asset's checksum never matches what's on disk and every
// reinstall flags it as updated forever.
func TestInstallerOpencodePersonaDelegateCollisionIsIdempotent(t *testing.T) {
	env, installer, adapter := setupOpencodeInstallTest(t)

	env.SetupAsset("personas", "bug-hunter.md", "---\ntai-schema-version: 1\nname: Bug Hunter\ndescription: A persona\n---\n\n# Persona body")
	env.SetupAsset("delegates", "bug-hunter.md", "---\ntai-schema-version: 1\nname: bug-hunter\ndescription: A delegate\n---\n\n# Delegate body")

	// Order matters: the delegate is listed after the persona, so "later wins"
	// means the delegate is the one that survives on disk and in the manifest.
	assets := []*config.Asset{
		{Type: config.AssetTypePersona, Name: "bug-hunter.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "personas", "bug-hunter.md")},
		{Type: config.AssetTypeDelegate, Name: "bug-hunter.md", SourceName: "local",
			SourcePath: filepath.Join(env.ConfigRepo, "assets", "delegates", "bug-hunter.md")},
	}

	result1, err := installer.Install(adapter, "opencode-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("First install failed: %v", err)
	}

	// Exactly one manifest entry for the colliding path - not two.
	collidingPath := "agents/bug-hunter.md"
	count := 0
	var entry ManifestEntry
	for _, e := range result1.Manifest.Files {
		if e.Path == collidingPath {
			count++
			entry = e
		}
	}
	if count != 1 {
		t.Fatalf("manifest has %d entries for %s, want 1", count, collidingPath)
	}
	// Later wins: the delegate is the survivor.
	if want := filepath.Join(env.ConfigRepo, "assets", "delegates", "bug-hunter.md"); entry.SourcePath != want {
		t.Errorf("surviving entry SourcePath = %q, want delegate %q", entry.SourcePath, want)
	}

	// Reinstalling identical assets must be a no-op: no phantom "updated".
	result2, err := installer.Install(adapter, "opencode-profile", assets, InstallOptions{})
	if err != nil {
		t.Fatalf("Second install failed: %v", err)
	}
	if len(result2.Updated) != 0 {
		t.Errorf("Second install: Updated = %v, want none", result2.Updated)
	}
	if len(result2.Added) != 0 {
		t.Errorf("Second install: Added = %v, want none", result2.Added)
	}
	hasUnchanged := false
	for _, p := range result2.Unchanged {
		if p == collidingPath {
			hasUnchanged = true
		}
	}
	if !hasUnchanged {
		t.Errorf("Second install: %s not reported unchanged (Unchanged = %v)", collidingPath, result2.Unchanged)
	}
}
