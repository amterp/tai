package profile

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/testutil"
)

func setupTestEnv(t *testing.T) (*testutil.TestEnv, map[string]*source.LoadedSource, map[string]*config.Bundle) {
	t.Helper()
	env := testutil.NewTestEnv(t)

	// Create assets
	env.SetupAsset("personas", "senior-swe.md", "# Senior SWE")
	env.SetupAsset("personas", "brainstormer.md", "# Brainstormer")
	env.SetupAsset("delegates", "code-reviewer.md", "# Code Reviewer")
	env.SetupAsset("files", "README.md", "# README")

	// Load sources
	loader := source.NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, err := loader.Load(src)
	if err != nil {
		t.Fatalf("failed to load source: %v", err)
	}
	sources := map[string]*source.LoadedSource{"local": ls}

	// Create bundles
	bundles := map[string]*config.Bundle{
		"core": {
			Name: "core",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/senior-swe.md"},
				{Type: "delegate", Path: "delegates/code-reviewer.md"},
			},
		},
		"extras": {
			Name: "extras",
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/brainstormer.md"},
				{Type: "file", Path: "files/README.md", Dest: ""},
			},
		},
	}

	return env, sources, bundles
}

// bundleRefs builds a `bundles:` list from bare names, the shape nearly every
// test uses.
func bundleRefs(names ...string) []config.BundleEntry {
	entries := make([]config.BundleEntry, len(names))
	for i, name := range names {
		entries[i] = config.BundleEntry{Name: name}
	}
	return entries
}

func TestResolverResolveSimple(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"work": {
			Name:    "work",
			Target:  "claude",
			Bundles: bundleRefs("core"),
		},
	}

	resolver := NewResolver(profiles, bundles, sources)
	resolved, err := resolver.Resolve("work")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Name != "work" {
		t.Errorf("Name = %q, want 'work'", resolved.Name)
	}
	if resolved.Target != "claude" {
		t.Errorf("Target = %q, want 'claude'", resolved.Target)
	}
	if len(resolved.Assets) != 2 {
		t.Errorf("len(Assets) = %d, want 2", len(resolved.Assets))
	}
}

func TestResolverResolveMultipleBundles(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"full": {
			Name:    "full",
			Target:  "claude",
			Bundles: bundleRefs("core", "extras"),
		},
	}

	resolver := NewResolver(profiles, bundles, sources)
	resolved, err := resolver.Resolve("full")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if len(resolved.Assets) != 4 {
		t.Errorf("len(Assets) = %d, want 4", len(resolved.Assets))
	}
}

func TestResolverResolveWithDirectAssets(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"mixed": {
			Name:    "mixed",
			Target:  "claude",
			Bundles: bundleRefs("core"),
			Assets: []config.AssetEntry{
				{Type: "file", Path: "files/README.md", Dest: ""},
			},
		},
	}

	resolver := NewResolver(profiles, bundles, sources)
	resolved, err := resolver.Resolve("mixed")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// 2 from core bundle + 1 direct
	if len(resolved.Assets) != 3 {
		t.Errorf("len(Assets) = %d, want 3", len(resolved.Assets))
	}
}

func TestResolverResolveWithOptions(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"custom": {
			Name:    "custom",
			Target:  "claude",
			Bundles: bundleRefs("core"),
			Options: map[string]any{
				"some_flag":     true,
				"another_value": "test",
			},
		},
	}

	resolver := NewResolver(profiles, bundles, sources)
	resolved, err := resolver.Resolve("custom")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if resolved.Options == nil {
		t.Fatal("Options is nil")
	}
	if v, ok := resolved.Options["some_flag"]; !ok || v != true {
		t.Errorf("Options[some_flag] = %v, want true", v)
	}
}

func TestResolverResolveOverrideLogging(t *testing.T) {
	env, sources, _ := setupTestEnv(t)

	// Create bundles with overlapping assets
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
				{Type: "persona", Path: "personas/senior-swe.md"}, // Same asset
			},
		},
	}

	// Reload source with updated asset
	env.SetupAsset("personas", "senior-swe.md", "# Updated SWE")
	loader := source.NewLoader(env.ConfigRepo)
	src := config.Source{Name: "local", Type: "local", Path: "./assets"}
	ls, _ := loader.Load(src)
	sources["local"] = ls

	profiles := map[string]*config.Profile{
		"test": {
			Name:    "test",
			Target:  "claude",
			Bundles: bundleRefs("base", "override"),
		},
	}

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	resolver := NewResolver(profiles, bundles, sources)
	resolver.SetLogger(logger)

	_, err := resolver.Resolve("test")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Check that override was logged
	logOutput := buf.String()
	if !strings.Contains(logOutput, "Override") {
		t.Errorf("expected override to be logged, got: %s", logOutput)
	}
}

func TestResolverResolveMissingProfile(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{}

	resolver := NewResolver(profiles, bundles, sources)
	_, err := resolver.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
}

func TestResolverResolveMissingBundle(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"bad": {
			Name:    "bad",
			Target:  "claude",
			Bundles: bundleRefs("nonexistent"),
		},
	}

	resolver := NewResolver(profiles, bundles, sources)
	_, err := resolver.Resolve("bad")
	if err == nil {
		t.Fatal("expected error for missing bundle, got nil")
	}
}

func TestResolverResolveMissingDirectAsset(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"bad": {
			Name:    "bad",
			Target:  "claude",
			Bundles: bundleRefs("core"),
			Assets: []config.AssetEntry{
				{Type: "persona", Path: "personas/nonexistent.md"},
			},
		},
	}

	resolver := NewResolver(profiles, bundles, sources)
	_, err := resolver.Resolve("bad")
	if err == nil {
		t.Fatal("expected error for missing asset, got nil")
	}
}

func TestResolverListProfiles(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"personal": {Name: "personal", Target: "claude", Bundles: bundleRefs("core")},
		"work":     {Name: "work", Target: "claude", Bundles: bundleRefs("core", "extras")},
	}

	resolver := NewResolver(profiles, bundles, sources)
	names := resolver.ListProfiles()

	if len(names) != 2 {
		t.Errorf("len(names) = %d, want 2", len(names))
	}

	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}
	if !found["personal"] || !found["work"] {
		t.Error("missing expected profile names")
	}
}

func TestResolverGetProfile(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"test": {Name: "test", Target: "claude", Bundles: bundleRefs("core")},
	}

	resolver := NewResolver(profiles, bundles, sources)

	// Existing profile
	p, ok := resolver.GetProfile("test")
	if !ok {
		t.Error("GetProfile returned false for existing profile")
	}
	if p.Name != "test" {
		t.Errorf("profile.Name = %q, want 'test'", p.Name)
	}

	// Non-existing profile
	_, ok = resolver.GetProfile("nonexistent")
	if ok {
		t.Error("GetProfile returned true for non-existing profile")
	}
}

func TestResolverPropagatesPostInstall(t *testing.T) {
	_, sources, bundles := setupTestEnv(t)

	profiles := map[string]*config.Profile{
		"hooked": {
			Name:               "hooked",
			Target:             "claude",
			Bundles:            bundleRefs("core"),
			PostInstall:        []string{"sh", "-c", "./notify.sh"},
			PostInstallTimeout: "90s",
		},
		"plain": {
			Name:    "plain",
			Target:  "claude",
			Bundles: bundleRefs("core"),
		},
	}

	resolver := NewResolver(profiles, bundles, sources)

	hooked, err := resolver.Resolve("hooked")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(hooked.PostInstall) != 3 || hooked.PostInstall[2] != "./notify.sh" {
		t.Errorf("PostInstall = %v, want propagated argv", hooked.PostInstall)
	}
	if hooked.PostInstallTimeout != 90*time.Second {
		t.Errorf("PostInstallTimeout = %v, want 90s", hooked.PostInstallTimeout)
	}

	plain, err := resolver.Resolve("plain")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(plain.PostInstall) != 0 {
		t.Errorf("PostInstall = %v, want empty", plain.PostInstall)
	}
	if plain.PostInstallTimeout != config.DefaultCommandTimeout {
		t.Errorf("PostInstallTimeout = %v, want default", plain.PostInstallTimeout)
	}
}
