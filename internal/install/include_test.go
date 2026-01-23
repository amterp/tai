package install

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/source"
)

func TestComputeDestPath(t *testing.T) {
	resolver := NewIncludeResolver(nil, nil)

	tests := []struct {
		name            string
		dest            string
		srcPath         string
		srcURL          string
		targetSkillPath string
		want            string
		wantErr         bool
	}{
		{
			name:            "directory dest preserves filename from path",
			dest:            "references/",
			srcPath:         "docs/SYNTAX.md",
			targetSkillPath: "/target/skill",
			want:            "/target/skill/references/SYNTAX.md",
		},
		{
			name:            "directory dest preserves filename from URL",
			dest:            "examples/",
			srcURL:          "https://example.com/path/to/hello.rad",
			targetSkillPath: "/target/skill",
			want:            "/target/skill/examples/hello.rad",
		},
		{
			name:            "explicit dest path",
			dest:            "docs/syntax-reference.md",
			srcPath:         "SYNTAX.md",
			targetSkillPath: "/target/skill",
			want:            "/target/skill/docs/syntax-reference.md",
		},
		{
			name:            "nested directory dest",
			dest:            "deep/nested/path/",
			srcPath:         "file.txt",
			targetSkillPath: "/target/skill",
			want:            "/target/skill/deep/nested/path/file.txt",
		},
		{
			name:            "path traversal in dest rejected",
			dest:            "../../../etc/passwd",
			srcPath:         "file.txt",
			targetSkillPath: "/target/skill",
			wantErr:         true,
		},
		{
			name:            "path traversal with directory dest rejected",
			dest:            "../../",
			srcPath:         "file.txt",
			targetSkillPath: "/target/skill",
			wantErr:         true,
		},
		{
			name:            "empty filename from URL rejected",
			dest:            "docs/",
			srcURL:          "https://example.com/",
			targetSkillPath: "/target/skill",
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.computeDestPath(tt.dest, tt.srcPath, tt.srcURL, tt.targetSkillPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("computeDestPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("computeDestPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveSourceInclude(t *testing.T) {
	// Create a test source directory
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "SYNTAX.md"), []byte("# Syntax Reference"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create subdirectory in source
	subDir := filepath.Join(srcDir, "examples")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "hello.rad"), []byte("print 'hello'"), 0644); err != nil {
		t.Fatal(err)
	}

	sources := map[string]*source.LoadedSource{
		"rad": {
			Source:       config.Source{Name: "rad", Path: srcDir},
			ResolvedPath: srcDir,
		},
	}

	resolver := NewIncludeResolver(sources, nil)

	t.Run("copy single file to directory dest", func(t *testing.T) {
		targetDir := t.TempDir()

		inc := config.Include{
			From: "rad",
			Path: "SYNTAX.md",
			Dest: "references/",
		}

		err := resolver.resolveSourceInclude(inc, targetDir)
		if err != nil {
			t.Fatalf("resolveSourceInclude() error = %v", err)
		}

		destPath := filepath.Join(targetDir, "references", "SYNTAX.md")
		content, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("Reading dest file: %v", err)
		}
		if string(content) != "# Syntax Reference" {
			t.Errorf("content = %q, want '# Syntax Reference'", string(content))
		}
	})

	t.Run("copy single file to explicit path", func(t *testing.T) {
		targetDir := t.TempDir()

		inc := config.Include{
			From: "rad",
			Path: "SYNTAX.md",
			Dest: "docs/syntax.md",
		}

		err := resolver.resolveSourceInclude(inc, targetDir)
		if err != nil {
			t.Fatalf("resolveSourceInclude() error = %v", err)
		}

		destPath := filepath.Join(targetDir, "docs", "syntax.md")
		content, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("Reading dest file: %v", err)
		}
		if string(content) != "# Syntax Reference" {
			t.Errorf("content = %q, want '# Syntax Reference'", string(content))
		}
	})

	t.Run("copy directory", func(t *testing.T) {
		targetDir := t.TempDir()

		inc := config.Include{
			From: "rad",
			Path: "examples",
			Dest: "scripts/",
		}

		err := resolver.resolveSourceInclude(inc, targetDir)
		if err != nil {
			t.Fatalf("resolveSourceInclude() error = %v", err)
		}

		destPath := filepath.Join(targetDir, "scripts", "examples", "hello.rad")
		content, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("Reading dest file: %v", err)
		}
		if string(content) != "print 'hello'" {
			t.Errorf("content = %q, want \"print 'hello'\"", string(content))
		}
	})

	t.Run("unknown source returns error", func(t *testing.T) {
		targetDir := t.TempDir()

		inc := config.Include{
			From: "unknown",
			Path: "SYNTAX.md",
			Dest: "references/",
		}

		err := resolver.resolveSourceInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveSourceInclude() should return error for unknown source")
		}
	})

	t.Run("missing file returns error", func(t *testing.T) {
		targetDir := t.TempDir()

		inc := config.Include{
			From: "rad",
			Path: "nonexistent.md",
			Dest: "references/",
		}

		err := resolver.resolveSourceInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveSourceInclude() should return error for missing file")
		}
	})

	t.Run("path traversal in source rejected", func(t *testing.T) {
		targetDir := t.TempDir()

		inc := config.Include{
			From: "rad",
			Path: "../../../etc/passwd",
			Dest: "references/",
		}

		err := resolver.resolveSourceInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveSourceInclude() should reject path traversal")
		}
		if err != nil && !strings.Contains(err.Error(), "path traversal") {
			t.Errorf("error should mention path traversal, got: %v", err)
		}
	})

	t.Run("path traversal in dest rejected", func(t *testing.T) {
		targetDir := t.TempDir()

		inc := config.Include{
			From: "rad",
			Path: "SYNTAX.md",
			Dest: "../../../etc/malicious.md",
		}

		err := resolver.resolveSourceInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveSourceInclude() should reject path traversal in dest")
		}
		if err != nil && !strings.Contains(err.Error(), "path traversal") {
			t.Errorf("error should mention path traversal, got: %v", err)
		}
	})

	t.Run("symlink in source rejected", func(t *testing.T) {
		// Create a symlink in the source
		symlinkSrcDir := t.TempDir()
		realFile := filepath.Join(symlinkSrcDir, "real.txt")
		if err := os.WriteFile(realFile, []byte("real content"), 0644); err != nil {
			t.Fatal(err)
		}
		symlinkPath := filepath.Join(symlinkSrcDir, "link.txt")
		if err := os.Symlink(realFile, symlinkPath); err != nil {
			t.Skip("cannot create symlink (possibly running on Windows without privileges)")
		}

		symlinkSources := map[string]*source.LoadedSource{
			"test": {
				Source:       config.Source{Name: "test", Path: symlinkSrcDir},
				ResolvedPath: symlinkSrcDir,
			},
		}
		symlinkResolver := NewIncludeResolver(symlinkSources, nil)

		targetDir := t.TempDir()
		inc := config.Include{
			From: "test",
			Path: "link.txt",
			Dest: "docs/",
		}

		err := symlinkResolver.resolveSourceInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveSourceInclude() should reject symlinks")
		}
		if err != nil && !strings.Contains(err.Error(), "symlink") {
			t.Errorf("error should mention symlink, got: %v", err)
		}
	})
}

func TestResolveURLInclude(t *testing.T) {
	t.Run("successful URL fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("# Content from URL"))
		}))
		defer server.Close()

		resolver := NewIncludeResolver(nil, server.Client())
		targetDir := t.TempDir()

		inc := config.Include{
			URL:  server.URL + "/file.md",
			Dest: "docs/remote.md",
		}

		err := resolver.resolveURLInclude(inc, targetDir)
		if err != nil {
			t.Fatalf("resolveURLInclude() error = %v", err)
		}

		destPath := filepath.Join(targetDir, "docs", "remote.md")
		content, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("Reading dest file: %v", err)
		}
		if string(content) != "# Content from URL" {
			t.Errorf("content = %q, want '# Content from URL'", string(content))
		}
	})

	t.Run("directory dest preserves URL filename", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("content"))
		}))
		defer server.Close()

		resolver := NewIncludeResolver(nil, server.Client())
		targetDir := t.TempDir()

		inc := config.Include{
			URL:  server.URL + "/path/to/hello.rad",
			Dest: "examples/",
		}

		err := resolver.resolveURLInclude(inc, targetDir)
		if err != nil {
			t.Fatalf("resolveURLInclude() error = %v", err)
		}

		destPath := filepath.Join(targetDir, "examples", "hello.rad")
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("Expected file at %s, but it doesn't exist", destPath)
		}
	})

	t.Run("HTTP error returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		resolver := NewIncludeResolver(nil, server.Client())
		targetDir := t.TempDir()

		inc := config.Include{
			URL:  server.URL + "/notfound.md",
			Dest: "docs/file.md",
		}

		err := resolver.resolveURLInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveURLInclude() should return error for HTTP 404")
		}
	})

	t.Run("invalid URL scheme rejected", func(t *testing.T) {
		resolver := NewIncludeResolver(nil, http.DefaultClient)
		targetDir := t.TempDir()

		inc := config.Include{
			URL:  "file:///etc/passwd",
			Dest: "docs/passwd",
		}

		err := resolver.resolveURLInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveURLInclude() should reject file:// scheme")
		}
		if err != nil && !strings.Contains(err.Error(), "scheme") {
			t.Errorf("error should mention scheme, got: %v", err)
		}
	})

	t.Run("path traversal in dest rejected", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("malicious content"))
		}))
		defer server.Close()

		resolver := NewIncludeResolver(nil, server.Client())
		targetDir := t.TempDir()

		inc := config.Include{
			URL:  server.URL + "/file.md",
			Dest: "../../../etc/malicious",
		}

		err := resolver.resolveURLInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveURLInclude() should reject path traversal in dest")
		}
		if err != nil && !strings.Contains(err.Error(), "path traversal") {
			t.Errorf("error should mention path traversal, got: %v", err)
		}
	})

	t.Run("oversized response rejected", func(t *testing.T) {
		// Create a server that returns more than MaxIncludeSize bytes
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Write slightly more than MaxIncludeSize
			data := make([]byte, MaxIncludeSize+1000)
			w.Write(data)
		}))
		defer server.Close()

		resolver := NewIncludeResolver(nil, server.Client())
		targetDir := t.TempDir()

		inc := config.Include{
			URL:  server.URL + "/large.bin",
			Dest: "docs/large.bin",
		}

		err := resolver.resolveURLInclude(inc, targetDir)
		if err == nil {
			t.Error("resolveURLInclude() should reject oversized response")
		}
		if err != nil && !strings.Contains(err.Error(), "exceeds maximum size") {
			t.Errorf("error should mention size limit, got: %v", err)
		}

		// Verify the oversized file was cleaned up
		destPath := filepath.Join(targetDir, "docs", "large.bin")
		if _, err := os.Stat(destPath); !os.IsNotExist(err) {
			t.Error("oversized file should have been cleaned up")
		}
	})
}

func TestResolveIncludes(t *testing.T) {
	// Create a test source directory
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "SYNTAX.md"), []byte("# Syntax"), 0644); err != nil {
		t.Fatal(err)
	}

	sources := map[string]*source.LoadedSource{
		"rad": {
			Source:       config.Source{Name: "rad", Path: srcDir},
			ResolvedPath: srcDir,
		},
	}

	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("# Remote Content"))
	}))
	defer server.Close()

	resolver := NewIncludeResolver(sources, server.Client())

	t.Run("resolves multiple includes", func(t *testing.T) {
		targetDir := t.TempDir()

		manifest := &config.SkillManifest{
			Include: []config.Include{
				{From: "rad", Path: "SYNTAX.md", Dest: "references/"},
				{URL: server.URL + "/remote.md", Dest: "docs/remote.md"},
			},
		}

		err := resolver.ResolveIncludes(manifest, targetDir)
		if err != nil {
			t.Fatalf("ResolveIncludes() error = %v", err)
		}

		// Check source-based include
		srcContent, err := os.ReadFile(filepath.Join(targetDir, "references", "SYNTAX.md"))
		if err != nil {
			t.Fatalf("Reading source include: %v", err)
		}
		if string(srcContent) != "# Syntax" {
			t.Errorf("source content = %q, want '# Syntax'", string(srcContent))
		}

		// Check URL-based include
		urlContent, err := os.ReadFile(filepath.Join(targetDir, "docs", "remote.md"))
		if err != nil {
			t.Fatalf("Reading URL include: %v", err)
		}
		if string(urlContent) != "# Remote Content" {
			t.Errorf("URL content = %q, want '# Remote Content'", string(urlContent))
		}
	})

	t.Run("empty manifest succeeds", func(t *testing.T) {
		targetDir := t.TempDir()

		manifest := &config.SkillManifest{
			Include: []config.Include{},
		}

		err := resolver.ResolveIncludes(manifest, targetDir)
		if err != nil {
			t.Fatalf("ResolveIncludes() error = %v", err)
		}
	})

	t.Run("error includes index", func(t *testing.T) {
		targetDir := t.TempDir()

		manifest := &config.SkillManifest{
			Include: []config.Include{
				{From: "rad", Path: "SYNTAX.md", Dest: "references/"},
				{From: "rad", Path: "nonexistent.md", Dest: "docs/"}, // This will fail
			},
		}

		err := resolver.ResolveIncludes(manifest, targetDir)
		if err == nil {
			t.Fatal("ResolveIncludes() should return error")
		}
		if !strings.Contains(err.Error(), "include[1]") {
			t.Errorf("error should include index, got: %v", err)
		}
	})
}

func TestCopyDirectoryPreservesPermissions(t *testing.T) {
	srcDir := t.TempDir()

	// Create a subdirectory with specific permissions
	subDir := filepath.Join(srcDir, "bin")
	if err := os.MkdirAll(subDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "run.sh"), []byte("#!/bin/bash"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("# Docs"), 0644); err != nil {
		t.Fatal(err)
	}

	dstDir := filepath.Join(t.TempDir(), "dest")
	resolver := NewIncludeResolver(nil, nil)
	if err := resolver.copyDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("copyDirectory failed: %v", err)
	}

	// Check file permissions
	scriptInfo, err := os.Stat(filepath.Join(dstDir, "bin", "run.sh"))
	if err != nil {
		t.Fatalf("stat script: %v", err)
	}
	if scriptInfo.Mode().Perm() != 0755 {
		t.Errorf("script permissions = %o, want %o", scriptInfo.Mode().Perm(), os.FileMode(0755))
	}

	docInfo, err := os.Stat(filepath.Join(dstDir, "README.md"))
	if err != nil {
		t.Fatalf("stat doc: %v", err)
	}
	if docInfo.Mode().Perm() != 0644 {
		t.Errorf("doc permissions = %o, want %o", docInfo.Mode().Perm(), os.FileMode(0644))
	}

	// Check directory permissions
	binInfo, err := os.Stat(filepath.Join(dstDir, "bin"))
	if err != nil {
		t.Fatalf("stat bin dir: %v", err)
	}
	if binInfo.Mode().Perm() != 0750 {
		t.Errorf("bin dir permissions = %o, want %o", binInfo.Mode().Perm(), os.FileMode(0750))
	}
}

func TestDefaultHTTPClient(t *testing.T) {
	client := DefaultHTTPClient()

	if client.Timeout != DefaultHTTPTimeout {
		t.Errorf("Timeout = %v, want %v", client.Timeout, DefaultHTTPTimeout)
	}

	if client.CheckRedirect == nil {
		t.Error("CheckRedirect should be configured")
	}
}
