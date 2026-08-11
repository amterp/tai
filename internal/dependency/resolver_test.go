package dependency

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
)

// testLogger captures log messages for test assertions.
type testLogger struct {
	messages []string
}

func (l *testLogger) Printf(format string, args ...any) {
	l.messages = append(l.messages, format)
}

func TestResolveNoDeps(t *testing.T) {
	// Setup temp directory with assets that have no depends
	tmpDir := t.TempDir()
	setupAsset(t, tmpDir, "delegates", "reviewer.md", `---
name: Reviewer
description: Reviews things
---

Content here.
`)

	sources := loadSources(t, tmpDir)
	resolver := NewResolver(sources)

	asset := &config.Asset{
		Type:       config.AssetTypeDelegate,
		Name:       "reviewer.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "delegates", "reviewer.md"),
	}

	result, err := resolver.Resolve([]*config.Asset{asset})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("len(result) = %d, want 1", len(result))
	}
	if result[0].Name != "reviewer.md" {
		t.Errorf("result[0].Name = %q, want %q", result[0].Name, "reviewer.md")
	}
}

func TestResolveSingleDep(t *testing.T) {
	tmpDir := t.TempDir()

	// Skill that depends on a delegate
	setupAsset(t, tmpDir, "skills", "code-review.md", `---
name: Code Review
description: Reviews code
depends:
  - delegates/bug-hunter.md
---

You are a code reviewer.
`)

	// The delegate it depends on
	setupAsset(t, tmpDir, "delegates", "bug-hunter.md", `---
name: Bug Hunter
description: Finds bugs
---

You find bugs.
`)

	sources := loadSources(t, tmpDir)
	resolver := NewResolver(sources)
	logger := &testLogger{}
	resolver.SetLogger(logger)

	skill := &config.Asset{
		Type:       config.AssetTypeSkill,
		Name:       "code-review.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "skills", "code-review.md"),
	}

	result, err := resolver.Resolve([]*config.Asset{skill})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should have both: dependency first, then the skill
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}

	// Dependency should come first
	if result[0].Type != config.AssetTypeDelegate || result[0].Name != "bug-hunter.md" {
		t.Errorf("result[0] = %s/%s, want delegate/bug-hunter.md",
			result[0].Type, result[0].Name)
	}

	// Original skill second
	if result[1].Type != config.AssetTypeSkill || result[1].Name != "code-review.md" {
		t.Errorf("result[1] = %s/%s, want skill/code-review.md",
			result[1].Type, result[1].Name)
	}

	// Should log the auto-include
	found := false
	for _, msg := range logger.messages {
		if strings.Contains(msg, "Auto-including") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected log message about auto-including dependency")
	}
}

func TestResolveTransitiveDeps(t *testing.T) {
	tmpDir := t.TempDir()

	// A -> B -> C
	setupAsset(t, tmpDir, "skills", "skill-a.md", `---
name: Skill A
depends:
  - delegates/delegate-b.md
---
`)

	setupAsset(t, tmpDir, "delegates", "delegate-b.md", `---
name: Delegate B
depends:
  - personas/persona-c.md
---
`)

	setupAsset(t, tmpDir, "personas", "persona-c.md", `---
name: Persona C
---
`)

	sources := loadSources(t, tmpDir)
	resolver := NewResolver(sources)

	skillA := &config.Asset{
		Type:       config.AssetTypeSkill,
		Name:       "skill-a.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "skills", "skill-a.md"),
	}

	result, err := resolver.Resolve([]*config.Asset{skillA})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should have all three in order: C, B, A (depth-first)
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// Deepest dependency first
	if result[0].Name != "persona-c.md" {
		t.Errorf("result[0].Name = %q, want persona-c.md", result[0].Name)
	}
	if result[1].Name != "delegate-b.md" {
		t.Errorf("result[1].Name = %q, want delegate-b.md", result[1].Name)
	}
	if result[2].Name != "skill-a.md" {
		t.Errorf("result[2].Name = %q, want skill-a.md", result[2].Name)
	}
}

func TestResolveCycleDetection(t *testing.T) {
	tmpDir := t.TempDir()

	// A -> B -> C -> A (cycle!)
	setupAsset(t, tmpDir, "delegates", "delegate-a.md", `---
name: A
depends:
  - delegates/delegate-b.md
---
`)

	setupAsset(t, tmpDir, "delegates", "delegate-b.md", `---
name: B
depends:
  - delegates/delegate-c.md
---
`)

	setupAsset(t, tmpDir, "delegates", "delegate-c.md", `---
name: C
depends:
  - delegates/delegate-a.md
---
`)

	sources := loadSources(t, tmpDir)
	resolver := NewResolver(sources)

	asset := &config.Asset{
		Type:       config.AssetTypeDelegate,
		Name:       "delegate-a.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "delegates", "delegate-a.md"),
	}

	_, err := resolver.Resolve([]*config.Asset{asset})
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("error = %q, should mention circular dependency", err.Error())
	}
}

func TestResolveExplicitOverridesDep(t *testing.T) {
	tmpDir := t.TempDir()

	// Skill depends on delegate
	setupAsset(t, tmpDir, "skills", "skill.md", `---
name: Skill
depends:
  - delegates/helper.md
---
`)

	setupAsset(t, tmpDir, "delegates", "helper.md", `---
name: Helper
---
`)

	sources := loadSources(t, tmpDir)
	resolver := NewResolver(sources)
	logger := &testLogger{}
	resolver.SetLogger(logger)

	// Both skill and delegate are explicitly included
	skill := &config.Asset{
		Type:       config.AssetTypeSkill,
		Name:       "skill.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "skills", "skill.md"),
	}

	delegate := &config.Asset{
		Type:       config.AssetTypeDelegate,
		Name:       "helper.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "delegates", "helper.md"),
	}

	result, err := resolver.Resolve([]*config.Asset{skill, delegate})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should still have 2 assets
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}

	// Should log that dep is already explicitly included
	found := false
	for _, msg := range logger.messages {
		if strings.Contains(msg, "already explicitly included") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected log about explicit inclusion")
	}
}

func TestResolveDiamondDeps(t *testing.T) {
	tmpDir := t.TempDir()

	// Diamond: A and B both depend on C
	//     A    B
	//      \  /
	//       C
	setupAsset(t, tmpDir, "skills", "skill-a.md", `---
name: A
depends:
  - delegates/shared.md
---
`)

	setupAsset(t, tmpDir, "skills", "skill-b.md", `---
name: B
depends:
  - delegates/shared.md
---
`)

	setupAsset(t, tmpDir, "delegates", "shared.md", `---
name: Shared
---
`)

	sources := loadSources(t, tmpDir)
	resolver := NewResolver(sources)

	skillA := &config.Asset{
		Type:       config.AssetTypeSkill,
		Name:       "skill-a.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "skills", "skill-a.md"),
	}

	skillB := &config.Asset{
		Type:       config.AssetTypeSkill,
		Name:       "skill-b.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "skills", "skill-b.md"),
	}

	result, err := resolver.Resolve([]*config.Asset{skillA, skillB})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should have 3 assets: shared (deduped), A, B
	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// Shared should only appear once
	sharedCount := 0
	for _, a := range result {
		if a.Name == "shared.md" {
			sharedCount++
		}
	}
	if sharedCount != 1 {
		t.Errorf("shared.md appears %d times, want 1", sharedCount)
	}
}

func TestResolveSkillDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Directory skill with SKILL.md
	skillDir := filepath.Join(tmpDir, "skills", "review-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(skillDir, "SKILL.md"), `---
name: Review Skill
depends:
  - delegates/helper.md
---

Skill content.
`)

	setupAsset(t, tmpDir, "delegates", "helper.md", `---
name: Helper
---
`)

	sources := loadSources(t, tmpDir)
	resolver := NewResolver(sources)

	skill := &config.Asset{
		Type:       config.AssetTypeSkill,
		Name:       "review-skill",
		SourceName: "local",
		SourcePath: skillDir,
		IsDir:      true,
	}

	result, err := resolver.Resolve([]*config.Asset{skill})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should have both: delegate first, then skill
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}

	if result[0].Name != "helper.md" {
		t.Errorf("result[0].Name = %q, want helper.md", result[0].Name)
	}
}

// A dependency path is a path from its source root, like every other asset
// reference. The asset type comes from the directory holding it, so a repo
// layout (assets/delegates/...) says "delegate" just as a bare one does.
func TestResolveDepUnderAssetsPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	setupAsset(t, filepath.Join(tmpDir, "assets"), "personas", "lead.md", `---
name: Lead
depends:
  - assets/delegates/helper.md
---
`)
	setupAsset(t, filepath.Join(tmpDir, "assets"), "delegates", "helper.md", `---
name: Helper
---
`)

	resolver := NewResolver(loadSources(t, tmpDir))
	persona := &config.Asset{
		Type:       config.AssetTypePersona,
		Name:       "lead.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "assets", "personas", "lead.md"),
	}

	result, err := resolver.Resolve([]*config.Asset{persona})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2 (the dependency was not pulled in)", len(result))
	}
	if result[0].Name != "helper.md" || result[0].Type != config.AssetTypeDelegate {
		t.Errorf("result[0] = %s/%s, want delegate/helper.md", result[0].Type, result[0].Name)
	}
}

func TestResolveDepWithoutTypeDirectoryErrors(t *testing.T) {
	tmpDir := t.TempDir()

	setupAsset(t, tmpDir, "personas", "lead.md", `---
name: Lead
depends:
  - helper.md
---
`)

	resolver := NewResolver(loadSources(t, tmpDir))
	persona := &config.Asset{
		Type:       config.AssetTypePersona,
		Name:       "lead.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "personas", "lead.md"),
	}

	_, err := resolver.Resolve([]*config.Asset{persona})
	if err == nil {
		t.Fatal("expected an error for a path with no asset type directory")
	}
	if !strings.Contains(err.Error(), "type-dir") {
		t.Errorf("error should say what shape a dependency path takes, got: %v", err)
	}
}

func TestResolveFileTypeNoDeps(t *testing.T) {
	tmpDir := t.TempDir()

	// File assets should never have deps extracted
	setupAsset(t, tmpDir, "files", "doc.md", `---
name: Doc
depends:
  - delegates/something.md
---

Content.
`)

	sources := loadSources(t, tmpDir)
	resolver := NewResolver(sources)

	asset := &config.Asset{
		Type:       config.AssetTypeFile,
		Name:       "doc.md",
		SourceName: "local",
		SourcePath: filepath.Join(tmpDir, "files", "doc.md"),
		Dest:       "",
	}

	result, err := resolver.Resolve([]*config.Asset{asset})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should only have the file, deps ignored
	if len(result) != 1 {
		t.Errorf("len(result) = %d, want 1 (file deps should be ignored)", len(result))
	}
}

// Helper functions

func setupAsset(t *testing.T, root, dir, name, content string) {
	t.Helper()
	assetDir := filepath.Join(root, dir)
	if err := os.MkdirAll(assetDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(assetDir, name), content)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func loadSources(t *testing.T, root string) map[string]*source.LoadedSource {
	t.Helper()
	loader := source.NewLoader(root)
	sources := []config.Source{
		{Name: "local", Type: "local", Path: root},
	}
	loaded, err := loader.LoadAll(sources)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	return loaded
}
