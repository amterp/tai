package bundle

import (
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/testutil"
)

func setupTestEnv(t *testing.T) (*testutil.TestEnv, map[string]*source.LoadedSource) {
	t.Helper()
	env := testutil.NewTestEnv(t)

	// Create assets
	env.SetupAsset("personas", "senior-swe.md", "# Senior SWE")
	env.SetupAsset("personas", "brainstormer.md", "# Brainstormer")
	env.SetupAsset("delegates", "code-reviewer.md", "# Code Reviewer")
	env.SetupAsset("docs", "README.md", "# README")

	// Load sources
	loader := source.NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatalf("failed to load source: %v", err)
	}

	return env, map[string]*source.LoadedSource{"local": ls}
}

func TestResolverResolveSimple(t *testing.T) {
	_, sources := setupTestEnv(t)

	bundles := map[string]*config.Bundle{
		"core": {
			Name: "core",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"},
				{Type: "delegate", Path: "delegates/code-reviewer.md"},
			},
		},
	}

	resolver := NewResolver(bundles, sources)
	resolved, err := resolver.Resolve("core")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Name != "core" {
		t.Errorf("Name = %q, want 'core'", resolved.Name)
	}
	if len(resolved.Assets) != 2 {
		t.Errorf("len(Assets) = %d, want 2", len(resolved.Assets))
	}
}

func TestResolverResolveWithIncludes(t *testing.T) {
	_, sources := setupTestEnv(t)

	bundles := map[string]*config.Bundle{
		"base": {
			Name: "base",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"},
			},
		},
		"extended": {
			Name:    "extended",
			Bundles: []string{"base"},
			Assets: []config.AssetEntry{
				{Type: "delegate", Path: "delegates/code-reviewer.md"},
			},
		},
	}

	resolver := NewResolver(bundles, sources)
	resolved, err := resolver.Resolve("extended")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should have assets from both bundles
	if len(resolved.Assets) != 2 {
		t.Errorf("len(Assets) = %d, want 2", len(resolved.Assets))
	}
}

func TestResolverResolveCircularReference(t *testing.T) {
	_, sources := setupTestEnv(t)

	bundles := map[string]*config.Bundle{
		"a": {
			Name:    "a",
			Bundles: []string{"b"},
		},
		"b": {
			Name:    "b",
			Bundles: []string{"c"},
		},
		"c": {
			Name:    "c",
			Bundles: []string{"a"}, // Circular!
		},
	}

	resolver := NewResolver(bundles, sources)
	_, err := resolver.Resolve("a")
	if err == nil {
		t.Fatal("expected error for circular reference, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("error should mention 'circular', got: %v", err)
	}
}

func TestResolverResolveDiamondDependency(t *testing.T) {
	_, sources := setupTestEnv(t)

	// Diamond: D includes B and C, both of which include A
	bundles := map[string]*config.Bundle{
		"a": {
			Name: "a",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"},
			},
		},
		"b": {
			Name:    "b",
			Bundles: []string{"a"},
			Assets: []config.AssetEntry{
				{Type: "delegate", Path: "delegates/code-reviewer.md"},
			},
		},
		"c": {
			Name:    "c",
			Bundles: []string{"a"},
			Assets: []config.AssetEntry{
				{Type: "doc", Path: "docs/README.md"},
			},
		},
		"d": {
			Name:    "d",
			Bundles: []string{"b", "c"},
		},
	}

	resolver := NewResolver(bundles, sources)
	resolved, err := resolver.Resolve("d")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should have 3 unique assets, not duplicated
	if len(resolved.Assets) != 3 {
		t.Errorf("len(Assets) = %d, want 3 (deduplicated)", len(resolved.Assets))
	}

	// Check no duplicates
	seen := make(map[string]bool)
	for _, asset := range resolved.Assets {
		key := asset.Type + "/" + asset.Name
		if seen[key] {
			t.Errorf("duplicate asset: %s", key)
		}
		seen[key] = true
	}
}

func TestResolverResolveMissingBundle(t *testing.T) {
	_, sources := setupTestEnv(t)

	bundles := map[string]*config.Bundle{}

	resolver := NewResolver(bundles, sources)
	_, err := resolver.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing bundle, got nil")
	}
}

func TestResolverResolveMissingAsset(t *testing.T) {
	_, sources := setupTestEnv(t)

	bundles := map[string]*config.Bundle{
		"bad": {
			Name: "bad",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/nonexistent.md"},
			},
		},
	}

	resolver := NewResolver(bundles, sources)
	_, err := resolver.Resolve("bad")
	if err == nil {
		t.Fatal("expected error for missing asset, got nil")
	}
}

func TestResolverResolveMissingSource(t *testing.T) {
	_, sources := setupTestEnv(t)

	bundles := map[string]*config.Bundle{
		"bad": {
			Name: "bad",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "unknown-source:personas/test.md"},
			},
		},
	}

	resolver := NewResolver(bundles, sources)
	_, err := resolver.Resolve("bad")
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
	if !strings.Contains(err.Error(), "source not found") {
		t.Errorf("error should mention 'source not found', got: %v", err)
	}
}

func TestResolverResolveMultiple(t *testing.T) {
	_, sources := setupTestEnv(t)

	bundles := map[string]*config.Bundle{
		"core": {
			Name: "core",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"},
			},
		},
		"extras": {
			Name: "extras",
			Assets: []config.AssetEntry{
				{Type: "delegate", Path: "delegates/code-reviewer.md"},
			},
		},
	}

	resolver := NewResolver(bundles, sources)
	resolved, overrides, err := resolver.ResolveMultiple([]string{"core", "extras"})
	if err != nil {
		t.Fatalf("ResolveMultiple failed: %v", err)
	}

	if len(resolved.Assets) != 2 {
		t.Errorf("len(Assets) = %d, want 2", len(resolved.Assets))
	}
	if len(overrides) != 0 {
		t.Errorf("len(overrides) = %d, want 0", len(overrides))
	}
}

func TestResolverResolveMultipleWithOverride(t *testing.T) {
	env, sources := setupTestEnv(t)

	// Create a second persona with the same name but different content
	env.SetupAsset("personas", "senior-swe.md", "# Senior SWE - Updated")

	// Reload sources to get the updated asset
	loader := source.NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	sources["local"] = ls

	bundles := map[string]*config.Bundle{
		"base": {
			Name: "base",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"},
			},
		},
		"override": {
			Name: "override",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"}, // Same asset, should override
			},
		},
	}

	resolver := NewResolver(bundles, sources)
	resolved, overrides, err := resolver.ResolveMultiple([]string{"base", "override"})
	if err != nil {
		t.Fatalf("ResolveMultiple failed: %v", err)
	}

	// Should have 1 asset (deduplicated)
	if len(resolved.Assets) != 1 {
		t.Errorf("len(Assets) = %d, want 1", len(resolved.Assets))
	}

	// Should have 1 override recorded
	if len(overrides) != 1 {
		t.Errorf("len(overrides) = %d, want 1", len(overrides))
	}
}

func TestResolverAssetOrder(t *testing.T) {
	_, sources := setupTestEnv(t)

	bundles := map[string]*config.Bundle{
		"ordered": {
			Name: "ordered",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"},
				{Type: "persona", Path: "personas/brainstormer.md"},
				{Type: "delegate", Path: "delegates/code-reviewer.md"},
			},
		},
	}

	resolver := NewResolver(bundles, sources)
	resolved, err := resolver.Resolve("ordered")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Check order is preserved
	expected := []string{"persona/senior-swe.md", "persona/brainstormer.md", "delegate/code-reviewer.md"}
	for i, asset := range resolved.Assets {
		actual := asset.Type + "/" + asset.Name
		if actual != expected[i] {
			t.Errorf("Asset[%d] = %q, want %q", i, actual, expected[i])
		}
	}
}
