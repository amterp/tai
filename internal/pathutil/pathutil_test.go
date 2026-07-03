package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"tilde prefix", "~/foo/bar", filepath.Join(home, "foo/bar"), false},
		{"no tilde", "/absolute/path", "/absolute/path", false},
		{"relative", "relative/path", "relative/path", false},
		{"tilde only", "~", "~", false}, // ~/  not just ~
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandHome(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandHome() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandHome() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsContainedIn(t *testing.T) {
	tests := []struct {
		name string
		path string
		base string
		want bool
	}{
		{"exact match", "/foo/bar", "/foo/bar", true},
		{"subdirectory", "/foo/bar/baz", "/foo/bar", true},
		{"parent", "/foo", "/foo/bar", false},
		{"sibling", "/foo/baz", "/foo/bar", false},
		{"traversal attempt", "/foo/bar/../baz", "/foo/bar", false},
		{"with dots in middle", "/foo/bar/./baz", "/foo/bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsContainedIn(tt.path, tt.base); got != tt.want {
				t.Errorf("IsContainedIn(%q, %q) = %v, want %v", tt.path, tt.base, got, tt.want)
			}
		})
	}
}

func TestMakeAbsolute(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		path    string
		base    string
		want    string
		wantErr bool
	}{
		{"already absolute", "/absolute/path", "/base", "/absolute/path", false},
		{"tilde path", "~/foo", "/base", filepath.Join(home, "foo"), false},
		{"relative with base", "relative", "/base", "/base/relative", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MakeAbsolute(tt.path, tt.base)
			if (err != nil) != tt.wantErr {
				t.Errorf("MakeAbsolute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MakeAbsolute() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		// Valid paths
		{"simple file", "file.md", false, ""},
		{"file in subdir", "subdir/file.md", false, ""},
		{"deep path", "a/b/c/d.txt", false, ""},
		{"dot in basename", "v1.0", false, ""},
		{"current dir prefix", "./file.md", false, ""},

		// Invalid: empty
		{"empty", "", true, "path is empty"},

		// Invalid: absolute paths
		{"absolute unix", "/etc/passwd", true, "absolute paths not allowed"},
		{"absolute with tilde", "~/file", false, ""}, // Not absolute in Go's view

		// Invalid: path traversal
		{"simple traversal", "../file", true, "path traversal not allowed"},
		{"deep traversal", "../../etc/passwd", true, "path traversal not allowed"},
		{"traversal in middle", "subdir/../../../etc/passwd", true, "path traversal not allowed"},
		{"just dotdot", "..", true, "path traversal not allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRelativePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRelativePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				if ipErr, ok := err.(*InvalidPathError); ok {
					if ipErr.Reason != tt.errMsg {
						t.Errorf("ValidateRelativePath(%q) reason = %q, want %q", tt.path, ipErr.Reason, tt.errMsg)
					}
				}
			}
		})
	}
}

func TestCopyDirPreservesPermissions(t *testing.T) {
	srcDir := t.TempDir()

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
	if err := CopyDir(srcDir, dstDir); err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}

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

	binInfo, err := os.Stat(filepath.Join(dstDir, "bin"))
	if err != nil {
		t.Fatalf("stat bin dir: %v", err)
	}
	if binInfo.Mode().Perm() != 0750 {
		t.Errorf("bin dir permissions = %o, want %o", binInfo.Mode().Perm(), os.FileMode(0750))
	}
}
