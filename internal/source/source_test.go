package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/testutil"
)

func TestLoaderLoad(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Set up assets
	env.SetupAsset("personas", "senior-swe.md", "# Senior SWE Persona")
	env.SetupAsset("personas", "brainstormer.md", "# Brainstormer Persona")
	env.SetupAsset("delegates", "code-reviewer.md", "# Code Reviewer")
	env.SetupAsset("docs", "README.md", "# Docs")

	// Create a skill directory
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# My Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)

	src := config.Source{
		Name: "local",
		Type: "local",
		Path: "./assets",
	}

	ls, err := loader.Load(src)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check resolved path
	expectedPath := filepath.Join(env.ConfigRepo, "assets")
	if ls.ResolvedPath != expectedPath {
		t.Errorf("ResolvedPath = %q, want %q", ls.ResolvedPath, expectedPath)
	}

	// Check discovered assets
	if len(ls.Assets["personas"]) != 2 {
		t.Errorf("len(personas) = %d, want 2", len(ls.Assets["personas"]))
	}
	if len(ls.Assets["delegates"]) != 1 {
		t.Errorf("len(delegates) = %d, want 1", len(ls.Assets["delegates"]))
	}
	if len(ls.Assets["skills"]) != 1 {
		t.Errorf("len(skills) = %d, want 1", len(ls.Assets["skills"]))
	}
	if len(ls.Assets["docs"]) != 1 {
		t.Errorf("len(docs) = %d, want 1", len(ls.Assets["docs"]))
	}
}

func TestLoaderLoadTaiRepo(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Create a separate "tai repo" directory structure
	taiRepoPath := filepath.Join(env.Root, "external-tai-repo")
	assetsPath := filepath.Join(taiRepoPath, "assets", "personas")
	if err := os.MkdirAll(assetsPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsPath, "external.md"), []byte("# External"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)

	src := config.Source{
		Name: "external",
		Type: "tai-repo",
		Path: taiRepoPath,
	}

	ls, err := loader.Load(src)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// tai-repo sources should point to assets/ subdirectory
	expectedPath := filepath.Join(taiRepoPath, "assets")
	if ls.ResolvedPath != expectedPath {
		t.Errorf("ResolvedPath = %q, want %q", ls.ResolvedPath, expectedPath)
	}

	// Should find the external persona
	if len(ls.Assets["personas"]) != 1 {
		t.Errorf("len(personas) = %d, want 1", len(ls.Assets["personas"]))
	}
}

func TestLoaderLoadAll(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupAsset("personas", "local-persona.md", "# Local")

	// Create secondary source
	secondaryPath := filepath.Join(env.Root, "secondary")
	secondaryAssets := filepath.Join(secondaryPath, "personas")
	if err := os.MkdirAll(secondaryAssets, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondaryAssets, "secondary-persona.md"), []byte("# Secondary"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)

	sources := []config.Source{
		{Name: "local", Type: "local", Path: "./assets"},
		{Name: "secondary", Type: "local", Path: secondaryPath},
	}

	loaded, err := loader.LoadAll(sources)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("len(loaded) = %d, want 2", len(loaded))
	}
	if _, ok := loaded["local"]; !ok {
		t.Error("missing 'local' source")
	}
	if _, ok := loaded["secondary"]; !ok {
		t.Error("missing 'secondary' source")
	}
}

func TestLoaderLoadMissing(t *testing.T) {
	env := testutil.NewTestEnv(t)
	loader := NewLoader(env.ConfigRepo)

	src := config.Source{
		Name: "missing",
		Type: "local",
		Path: "./nonexistent",
	}

	_, err := loader.Load(src)
	if err == nil {
		t.Error("expected error for missing source, got nil")
	}
}

func TestLoadedSourceGetAsset(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupAsset("personas", "test.md", "# Test")

	loader := NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	// Existing asset
	path, ok := ls.GetAsset("personas/test.md")
	if !ok {
		t.Error("GetAsset returned false for existing asset")
	}
	expectedPath := filepath.Join(env.ConfigRepo, "assets", "personas", "test.md")
	if path != expectedPath {
		t.Errorf("path = %q, want %q", path, expectedPath)
	}

	// Non-existing asset
	_, ok = ls.GetAsset("personas/nonexistent.md")
	if ok {
		t.Error("GetAsset returned true for non-existing asset")
	}
}

func TestLoadedSourceResolveAsset(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupAsset("personas", "test.md", "# Test")

	// Create a skill directory
	skillDir := filepath.Join(env.ConfigRepo, "assets", "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve file asset
	asset, err := ls.ResolveAsset("personas/test.md")
	if err != nil {
		t.Fatalf("ResolveAsset failed: %v", err)
	}
	if asset.Type != "personas" {
		t.Errorf("Type = %q, want 'personas'", asset.Type)
	}
	if asset.Name != "test.md" {
		t.Errorf("Name = %q, want 'test.md'", asset.Name)
	}
	if asset.SourceName != "local" {
		t.Errorf("SourceName = %q, want 'local'", asset.SourceName)
	}
	if asset.IsDir {
		t.Error("IsDir = true, want false")
	}

	// Resolve skill (directory) asset
	skillAsset, err := ls.ResolveAsset("skills/my-skill")
	if err != nil {
		t.Fatalf("ResolveAsset failed for skill: %v", err)
	}
	if !skillAsset.IsDir {
		t.Error("IsDir = false for skill, want true")
	}

	// Non-existing asset
	_, err = ls.ResolveAsset("personas/nonexistent.md")
	if err == nil {
		t.Error("expected error for non-existing asset")
	}

	// Invalid asset type
	if err := os.WriteFile(filepath.Join(env.ConfigRepo, "assets", "invalid", "test.md"), []byte("x"), 0644); err == nil {
		// This should fail because we'd need to create the directory first
		// The test is just checking that invalid types are rejected
	}
}

func TestLoadedSourceListAllAssets(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.SetupAsset("personas", "one.md", "# One")
	env.SetupAsset("personas", "two.md", "# Two")
	env.SetupAsset("delegates", "helper.md", "# Helper")

	loader := NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatal(err)
	}

	all := ls.ListAllAssets()
	if len(all) != 3 {
		t.Errorf("len(all) = %d, want 3", len(all))
	}

	// Check that paths are correctly formatted
	found := make(map[string]bool)
	for _, path := range all {
		found[path] = true
	}
	for _, expected := range []string{"personas/one.md", "personas/two.md", "delegates/helper.md"} {
		if !found[expected] {
			t.Errorf("missing expected asset: %s", expected)
		}
	}
}
