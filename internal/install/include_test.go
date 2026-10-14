package install

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/fetch"
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

	t.Run("copy directory without trailing slash places its contents at dest", func(t *testing.T) {
		targetDir := t.TempDir()

		inc := config.Include{
			From: "rad",
			Path: "examples/",
			Dest: "scripts",
		}

		err := resolver.resolveSourceInclude(inc, targetDir)
		if err != nil {
			t.Fatalf("resolveSourceInclude() error = %v", err)
		}

		if _, err := os.Stat(filepath.Join(targetDir, "scripts", "hello.rad")); err != nil {
			t.Errorf("want scripts/hello.rad: %v", err)
		}
		if _, err := os.Stat(filepath.Join(targetDir, "scripts", "examples")); !os.IsNotExist(err) {
			t.Errorf("scripts/examples exists, want the directory's contents directly under scripts/")
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
			_, _ = w.Write([]byte("# Content from URL"))
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
			_, _ = w.Write([]byte("content"))
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
			_, _ = w.Write([]byte("malicious content"))
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
		// Create a server that returns more than fetch.MaxBytes bytes
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Write slightly more than fetch.MaxBytes
			data := make([]byte, fetch.MaxBytes+1000)
			_, _ = w.Write(data)
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
		_, _ = w.Write([]byte("# Remote Content"))
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

		_, err := resolver.ResolveIncludes(manifest, targetDir)
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

		_, err := resolver.ResolveIncludes(manifest, targetDir)
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

		_, err := resolver.ResolveIncludes(manifest, targetDir)
		if err == nil {
			t.Fatal("ResolveIncludes() should return error")
		}
		if !strings.Contains(err.Error(), "include[1]") {
			t.Errorf("error should include index, got: %v", err)
		}
	})
}

func TestResolveCommandInclude(t *testing.T) {
	// Skip on Windows: these tests rely on /bin/sh.
	if runtime.GOOS == "windows" {
		t.Skip("command include tests require POSIX sh")
	}

	t.Run("happy path - single file", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)

		inc := config.Include{
			Command: []string{"sh", "-c", `printf 'hello world' > "$TAI_DEST_DIR/out.md"`},
			Dest:    "docs/",
		}

		if _, err := resolver.resolveCommandInclude(inc, targetDir); err != nil {
			t.Fatalf("resolveCommandInclude() error = %v", err)
		}

		content, err := os.ReadFile(filepath.Join(targetDir, "docs", "out.md"))
		if err != nil {
			t.Fatalf("reading dest file: %v", err)
		}
		if string(content) != "hello world" {
			t.Errorf("content = %q, want %q", string(content), "hello world")
		}
	})

	t.Run("happy path - multiple files in nested dirs", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)

		inc := config.Include{
			Command: []string{"sh", "-c", `
				mkdir -p "$TAI_DEST_DIR/sub"
				printf 'a' > "$TAI_DEST_DIR/a.txt"
				printf 'b' > "$TAI_DEST_DIR/sub/b.txt"
			`},
			Dest: "refs/",
		}

		if _, err := resolver.resolveCommandInclude(inc, targetDir); err != nil {
			t.Fatalf("resolveCommandInclude() error = %v", err)
		}

		for _, rel := range []string{"refs/a.txt", "refs/sub/b.txt"} {
			if _, err := os.Stat(filepath.Join(targetDir, rel)); err != nil {
				t.Errorf("expected %s, got: %v", rel, err)
			}
		}
	})

	t.Run("empty output returns error", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)

		inc := config.Include{
			Command: []string{"sh", "-c", "true"}, // exits 0 but writes nothing
			Dest:    "docs/",
		}

		_, err := resolver.resolveCommandInclude(inc, targetDir)
		if err == nil {
			t.Fatal("expected error for empty output")
		}
		if !strings.Contains(err.Error(), "no files") {
			t.Errorf("error should mention 'no files', got: %v", err)
		}
	})

	t.Run("nonzero exit returns error with stderr", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)

		// Use a binary that doesn't echo the exit code in its argv so the
		// assertion can't pass incidentally via the argv being printed.
		inc := config.Include{
			Command: []string{"sh", "-c", `printf 'boom on stderr' >&2; exit 7`},
			Dest:    "docs/",
		}

		_, err := resolver.resolveCommandInclude(inc, targetDir)
		if err == nil {
			t.Fatal("expected error for nonzero exit")
		}
		if !strings.Contains(err.Error(), "exited 7") {
			t.Errorf("error should mention 'exited 7', got: %v", err)
		}
		// Ensure the exit code isn't duplicated (we used to render it twice
		// via both the wrapped error and an explicit suffix).
		if strings.Count(err.Error(), "exit status 7") > 0 {
			t.Errorf("error should not include both 'exited 7' and 'exit status 7' (duplication), got: %v", err)
		}
		if !strings.Contains(err.Error(), "boom on stderr") {
			t.Errorf("error should include stderr tail, got: %v", err)
		}
		if !strings.Contains(err.Error(), "staging:") {
			t.Errorf("error should include staging dir path, got: %v", err)
		}
	})

	t.Run("timeout returns error and preserves staging", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)

		inc := config.Include{
			Command: []string{"sh", "-c", "sleep 5"},
			Dest:    "docs/",
			Timeout: "100ms",
		}

		_, err := resolver.resolveCommandInclude(inc, targetDir)
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("error should mention timeout, got: %v", err)
		}
		if !strings.Contains(err.Error(), "staging:") {
			t.Errorf("error should include staging dir path, got: %v", err)
		}
	})

	t.Run("env vars are set", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)
		resolver.SetCommandContext(CommandContext{
			AssetType:   "skill",
			AssetName:   "my-skill",
			TargetName:  "claude",
			ManifestDir: "/some/manifest/dir",
			Version:     "1.2.3",
		})

		// Write a probe script that dumps a few env vars into TAI_DEST_DIR.
		inc := config.Include{
			Command: []string{"sh", "-c", `
				printf '%s' "$TAI_ASSET_TYPE" > "$TAI_DEST_DIR/asset_type"
				printf '%s' "$TAI_ASSET_NAME" > "$TAI_DEST_DIR/asset_name"
				printf '%s' "$TAI_TARGET"     > "$TAI_DEST_DIR/target"
				printf '%s' "$TAI_MANIFEST_DIR" > "$TAI_DEST_DIR/manifest_dir"
				printf '%s' "$TAI_VERSION"    > "$TAI_DEST_DIR/version"
				test -n "$TAI_DEST_DIR"       # ensure it's set
			`},
			Dest: "out/",
		}

		if _, err := resolver.resolveCommandInclude(inc, targetDir); err != nil {
			t.Fatalf("resolveCommandInclude() error = %v", err)
		}

		expect := map[string]string{
			"asset_type":   "skill",
			"asset_name":   "my-skill",
			"target":       "claude",
			"manifest_dir": "/some/manifest/dir",
			"version":      "1.2.3",
		}
		for f, want := range expect {
			got, err := os.ReadFile(filepath.Join(targetDir, "out", f))
			if err != nil {
				t.Fatalf("reading %s: %v", f, err)
			}
			if string(got) != want {
				t.Errorf("%s = %q, want %q", f, string(got), want)
			}
		}
	})

	t.Run("atomic swap replaces existing dest", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)

		// Pre-existing dest with stale content.
		preExisting := filepath.Join(targetDir, "docs")
		if err := os.MkdirAll(preExisting, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(preExisting, "stale.md"), []byte("stale"), 0644); err != nil {
			t.Fatal(err)
		}

		inc := config.Include{
			Command: []string{"sh", "-c", `printf 'fresh' > "$TAI_DEST_DIR/fresh.md"`},
			Dest:    "docs/",
		}

		if _, err := resolver.resolveCommandInclude(inc, targetDir); err != nil {
			t.Fatalf("resolveCommandInclude() error = %v", err)
		}

		// stale.md should be gone, fresh.md should exist.
		if _, err := os.Stat(filepath.Join(preExisting, "stale.md")); !os.IsNotExist(err) {
			t.Errorf("stale.md should be removed, stat err = %v", err)
		}
		if _, err := os.Stat(filepath.Join(preExisting, "fresh.md")); err != nil {
			t.Errorf("fresh.md should exist, err = %v", err)
		}
	})

	t.Run("failure leaves pre-existing dest untouched", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)

		preExisting := filepath.Join(targetDir, "docs")
		if err := os.MkdirAll(preExisting, 0755); err != nil {
			t.Fatal(err)
		}
		original := []byte("original")
		if err := os.WriteFile(filepath.Join(preExisting, "keep.md"), original, 0644); err != nil {
			t.Fatal(err)
		}

		inc := config.Include{
			Command: []string{"sh", "-c", "exit 1"},
			Dest:    "docs/",
		}

		if _, err := resolver.resolveCommandInclude(inc, targetDir); err == nil {
			t.Fatal("expected error from failing script")
		}

		got, err := os.ReadFile(filepath.Join(preExisting, "keep.md"))
		if err != nil {
			t.Fatalf("reading preserved file: %v", err)
		}
		if string(got) != string(original) {
			t.Errorf("pre-existing content should be untouched on failure, got %q", string(got))
		}
	})

	t.Run("path traversal in dest rejected", func(t *testing.T) {
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)

		inc := config.Include{
			Command: []string{"sh", "-c", `printf 'x' > "$TAI_DEST_DIR/out"`},
			Dest:    "../../../etc/malicious",
		}

		_, err := resolver.resolveCommandInclude(inc, targetDir)
		if err == nil {
			t.Fatal("expected path traversal rejection")
		}
		if !strings.Contains(err.Error(), "path traversal") {
			t.Errorf("error should mention path traversal, got: %v", err)
		}
	})

	t.Run("dest at skill root merges into the skill", func(t *testing.T) {
		for _, dest := range []string{".", "./", "/"} {
			targetDir := t.TempDir()
			resolver := NewIncludeResolver(nil, nil)

			if err := os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("old"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(targetDir, "keep.md"), []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}

			inc := config.Include{
				Command: []string{"sh", "-c", `printf 'new' > "$TAI_DEST_DIR/SKILL.md" && mkdir "$TAI_DEST_DIR/refs" && printf 'r' > "$TAI_DEST_DIR/refs/r.md"`},
				Dest:    dest,
			}
			if _, err := resolver.resolveCommandInclude(inc, targetDir); err != nil {
				t.Fatalf("dest %q: resolveCommandInclude() error = %v", dest, err)
			}

			for file, want := range map[string]string{"SKILL.md": "new", "keep.md": "keep", "refs/r.md": "r"} {
				if got := string(mustReadFile(t, filepath.Join(targetDir, file))); got != want {
					t.Errorf("dest %q: %s = %q, want %q", dest, file, got, want)
				}
			}
		}
	})

	t.Run("symlink in a root merge is rejected", func(t *testing.T) {
		targetDir := t.TempDir()
		outside := filepath.Join(t.TempDir(), "victim.md")
		if err := os.WriteFile(outside, []byte("untouched"), 0644); err != nil {
			t.Fatal(err)
		}
		resolver := NewIncludeResolver(nil, nil)
		inc := config.Include{
			Command: []string{"sh", "-c", `ln -s "` + outside + `" "$TAI_DEST_DIR/SKILL.md"`},
			Dest:    "./",
		}
		if _, err := resolver.resolveCommandInclude(inc, targetDir); err == nil {
			t.Fatal("expected a symlink to be rejected")
		}
		if _, err := os.Lstat(filepath.Join(targetDir, "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("symlink was merged into the skill, lstat err = %v", err)
		}
	})

	t.Run("empty dest rejected by runtime", func(t *testing.T) {
		// Validate() would normally reject this; the resolver guards it too,
		// for callers that skip Validate().
		resolver := NewIncludeResolver(nil, nil)
		inc := config.Include{
			Command: []string{"sh", "-c", `printf 'x' > "$TAI_DEST_DIR/out"`},
		}
		if _, err := resolver.resolveCommandInclude(inc, t.TempDir()); err == nil {
			t.Error("empty dest should be rejected at runtime")
		}
	})

	t.Run("TAI_DEST_DIR from inherited env is overridden", func(t *testing.T) {
		// Some users may have a stale TAI_DEST_DIR exported in their shell;
		// the resolver must strip TAI_* from the inherited env so its own
		// authoritative value is the only one visible to the script.
		t.Setenv("TAI_DEST_DIR", "/should/not/win")
		t.Setenv("TAI_ASSET_NAME", "stale-name")

		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)
		resolver.SetCommandContext(CommandContext{AssetName: "real-name"})

		inc := config.Include{
			Command: []string{"sh", "-c", `
				printf '%s' "$TAI_DEST_DIR"   > "$TAI_DEST_DIR/dest"
				printf '%s' "$TAI_ASSET_NAME" > "$TAI_DEST_DIR/name"
			`},
			Dest: "out/",
		}

		if _, err := resolver.resolveCommandInclude(inc, targetDir); err != nil {
			t.Fatalf("resolveCommandInclude() error = %v", err)
		}

		destEcho := string(mustReadFile(t, filepath.Join(targetDir, "out", "dest")))
		if destEcho == "/should/not/win" {
			t.Errorf("inherited TAI_DEST_DIR leaked through, got %q", destEcho)
		}
		nameEcho := string(mustReadFile(t, filepath.Join(targetDir, "out", "name")))
		if nameEcho != "real-name" {
			t.Errorf("TAI_ASSET_NAME = %q, want 'real-name' (inherited value leaked)", nameEcho)
		}
	})

	t.Run("all TAI_ env vars are set even when context is zero", func(t *testing.T) {
		// Scripts running under `set -u` reference these unconditionally;
		// the resolver must always set them, even to empty string.
		targetDir := t.TempDir()
		resolver := NewIncludeResolver(nil, nil)
		// No SetCommandContext call - cmdCtx is the zero value.

		inc := config.Include{
			Command: []string{"sh", "-c", `
				set -u
				: "$TAI_DEST_DIR" "$TAI_ASSET_TYPE" "$TAI_ASSET_NAME"
				: "$TAI_TARGET"   "$TAI_MANIFEST_DIR" "$TAI_VERSION"
				printf 'ok' > "$TAI_DEST_DIR/marker"
			`},
			Dest: "out/",
		}

		if _, err := resolver.resolveCommandInclude(inc, targetDir); err != nil {
			t.Fatalf("resolveCommandInclude() error = %v\n(set -u under zero context should still see all TAI_* vars)", err)
		}
	})
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}
