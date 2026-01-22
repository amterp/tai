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
