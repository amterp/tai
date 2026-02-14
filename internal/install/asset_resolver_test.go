package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
)

func setupResolverTest(t *testing.T) (string, *AssetContentResolver) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create a source directory with assets
	srcDir := filepath.Join(tmpDir, "source")
	for _, dir := range []string{
		filepath.Join(srcDir, "personas"),
		filepath.Join(srcDir, "delegates"),
		filepath.Join(srcDir, "skills"),
		filepath.Join(srcDir, "files"),
		filepath.Join(srcDir, "statuslines"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create test assets
	writeFile(t, filepath.Join(srcDir, "personas", "test.md"), `---
name: Test Persona
description: A test persona
---

You are a test persona.
`)
	writeFile(t, filepath.Join(srcDir, "files", "plain.md"), "# Plain File\nNo frontmatter here.\n")
	writeFile(t, filepath.Join(srcDir, "statuslines", "config.yaml"), "level: minimal\n")

	loader := source.NewLoader(tmpDir)
	sources, err := loader.LoadAll([]config.Source{
		{Name: "local", Path: srcDir},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Convert to map[string]*source.LoadedSource
	resolver := NewAssetContentResolver(sources)
	return srcDir, resolver
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestAssetContentResolverPersona(t *testing.T) {
	_, resolver := setupResolverTest(t)

	content, err := resolver.ResolveAssetContent("local:personas/test.md")
	if err != nil {
		t.Fatalf("ResolveAssetContent failed: %v", err)
	}

	// Should return body only, frontmatter stripped
	if strings.Contains(string(content), "name: Test Persona") {
		t.Error("frontmatter should be stripped")
	}
	if !strings.Contains(string(content), "You are a test persona.") {
		t.Error("body content should be present")
	}
}

func TestAssetContentResolverPlainFile(t *testing.T) {
	_, resolver := setupResolverTest(t)

	content, err := resolver.ResolveAssetContent("local:files/plain.md")
	if err != nil {
		t.Fatalf("ResolveAssetContent failed: %v", err)
	}

	expected := "# Plain File\nNo frontmatter here.\n"
	if string(content) != expected {
		t.Errorf("content = %q, want %q", string(content), expected)
	}
}

func TestAssetContentResolverMissingAsset(t *testing.T) {
	_, resolver := setupResolverTest(t)

	_, err := resolver.ResolveAssetContent("local:personas/nonexistent.md")
	if err == nil {
		t.Fatal("expected error for missing asset")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, should contain 'not found'", err.Error())
	}
}

func TestAssetContentResolverMissingSource(t *testing.T) {
	_, resolver := setupResolverTest(t)

	_, err := resolver.ResolveAssetContent("nosource:personas/test.md")
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if !strings.Contains(err.Error(), "source") && !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, should mention source not found", err.Error())
	}
}

func TestAssetContentResolverRejectsConfigAssets(t *testing.T) {
	_, resolver := setupResolverTest(t)

	_, err := resolver.ResolveAssetContent("local:statuslines/config.yaml")
	if err == nil {
		t.Fatal("expected error for config asset type")
	}
	if !strings.Contains(err.Error(), "cannot include") {
		t.Errorf("error = %q, should say cannot include", err.Error())
	}
}

func TestAssetContentResolverDefaultSource(t *testing.T) {
	_, resolver := setupResolverTest(t)

	// Without "source:" prefix, defaults to "local"
	content, err := resolver.ResolveAssetContent("personas/test.md")
	if err != nil {
		t.Fatalf("ResolveAssetContent failed: %v", err)
	}

	if !strings.Contains(string(content), "You are a test persona.") {
		t.Error("body content should be present with default source")
	}
}

func TestAssetContentResolverInvalidPath(t *testing.T) {
	_, resolver := setupResolverTest(t)

	// Path without a recognized type directory prefix
	_, err := resolver.ResolveAssetContent("local:nodir.md")
	if err == nil {
		t.Fatal("expected error for path without type prefix")
	}
	if !strings.Contains(err.Error(), "cannot infer") {
		t.Errorf("error = %q, should mention cannot infer", err.Error())
	}
}

func TestAssetContentResolverRejectsBinaryContent(t *testing.T) {
	srcDir, resolver := setupResolverTest(t)

	// Write a file containing binary content (invalid UTF-8)
	binaryContent := []byte{0x00, 0xFF, 0xFE, 0x80, 0x81}
	writeFile(t, filepath.Join(srcDir, "files", "binary.bin"), string(binaryContent))

	_, err := resolver.ResolveAssetContent("local:files/binary.bin")
	if err == nil {
		t.Fatal("expected error for binary content")
	}
	if !strings.Contains(err.Error(), "binary content") {
		t.Errorf("error = %q, should mention binary content", err.Error())
	}
}
