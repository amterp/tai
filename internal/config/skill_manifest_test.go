package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIncludeValidate(t *testing.T) {
	tests := []struct {
		name    string
		include Include
		wantErr bool
	}{
		{
			name:    "valid source-based include",
			include: Include{From: "rad", Path: "SYNTAX.md", Dest: "references/"},
			wantErr: false,
		},
		{
			name:    "valid URL-based include",
			include: Include{URL: "https://example.com/file.md", Dest: "docs/file.md"},
			wantErr: false,
		},
		{
			name:    "empty include",
			include: Include{},
			wantErr: true,
		},
		{
			name:    "from without path",
			include: Include{From: "rad", Dest: "references/"},
			wantErr: true,
		},
		{
			name:    "path without from",
			include: Include{Path: "SYNTAX.md", Dest: "references/"},
			wantErr: true,
		},
		{
			name:    "url without dest",
			include: Include{URL: "https://example.com/file.md"},
			wantErr: true,
		},
		{
			name:    "from without dest",
			include: Include{From: "rad", Path: "SYNTAX.md"},
			wantErr: true,
		},
		{
			name:    "url and from both set",
			include: Include{URL: "https://example.com/file.md", From: "rad", Path: "foo", Dest: "bar"},
			wantErr: true,
		},
		{
			name:    "valid command include",
			include: Include{Command: []string{"echo", "hello"}, Dest: "docs/"},
			wantErr: false,
		},
		{
			name:    "command with explicit timeout",
			include: Include{Command: []string{"sh", "-c", "true"}, Dest: "out/", Timeout: "5s"},
			wantErr: false,
		},
		{
			name:    "command without dest",
			include: Include{Command: []string{"echo", "hello"}},
			wantErr: true,
		},
		{
			name:    "empty command list",
			include: Include{Command: []string{}, Dest: "docs/"},
			wantErr: true,
		},
		{
			name:    "command with empty arg",
			include: Include{Command: []string{"echo", ""}, Dest: "docs/"},
			wantErr: true,
		},
		{
			name:    "command and url both set",
			include: Include{Command: []string{"echo"}, URL: "https://example.com/x", Dest: "docs/"},
			wantErr: true,
		},
		{
			name:    "command and from both set",
			include: Include{Command: []string{"echo"}, From: "rad", Path: "x", Dest: "docs/"},
			wantErr: true,
		},
		{
			name:    "invalid timeout string",
			include: Include{Command: []string{"echo"}, Dest: "docs/", Timeout: "not-a-duration"},
			wantErr: true,
		},
		{
			name:    "zero timeout",
			include: Include{Command: []string{"echo"}, Dest: "docs/", Timeout: "0s"},
			wantErr: true,
		},
		{
			name:    "negative timeout",
			include: Include{Command: []string{"echo"}, Dest: "docs/", Timeout: "-1s"},
			wantErr: true,
		},
		{
			name:    "timeout without command",
			include: Include{URL: "https://example.com/file.md", Dest: "docs/file.md", Timeout: "5s"},
			wantErr: true,
		},
		{
			name:    "command dest cannot be skill root .",
			include: Include{Command: []string{"echo"}, Dest: "."},
			wantErr: true,
		},
		{
			name:    "command dest cannot be skill root ./",
			include: Include{Command: []string{"echo"}, Dest: "./"},
			wantErr: true,
		},
		{
			name:    "command dest cannot be skill root /",
			include: Include{Command: []string{"echo"}, Dest: "/"},
			wantErr: true,
		},
		{
			name:    "command include rejects fields",
			include: Include{Command: []string{"echo"}, Dest: "docs/", Fields: map[string]string{"name": "title"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.include.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Include.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIncludeIsURLBased(t *testing.T) {
	tests := []struct {
		name    string
		include Include
		want    bool
	}{
		{
			name:    "URL-based",
			include: Include{URL: "https://example.com/file.md", Dest: "docs/"},
			want:    true,
		},
		{
			name:    "source-based",
			include: Include{From: "rad", Path: "SYNTAX.md", Dest: "references/"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.include.IsURLBased(); got != tt.want {
				t.Errorf("Include.IsURLBased() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIncludeIsCommandBased(t *testing.T) {
	tests := []struct {
		name    string
		include Include
		want    bool
	}{
		{
			name:    "command-based",
			include: Include{Command: []string{"echo", "hi"}, Dest: "docs/"},
			want:    true,
		},
		{
			name:    "URL-based",
			include: Include{URL: "https://example.com/file.md", Dest: "docs/"},
			want:    false,
		},
		{
			name:    "source-based",
			include: Include{From: "rad", Path: "SYNTAX.md", Dest: "references/"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.include.IsCommandBased(); got != tt.want {
				t.Errorf("Include.IsCommandBased() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIncludeResolvedTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
		want    time.Duration
	}{
		{name: "default when empty", timeout: "", want: DefaultCommandIncludeTimeout},
		{name: "explicit 5s", timeout: "5s", want: 5 * time.Second},
		{name: "explicit 2m", timeout: "2m", want: 2 * time.Minute},
		{name: "malformed falls back to default", timeout: "not-a-duration", want: DefaultCommandIncludeTimeout},
		{name: "zero falls back to default", timeout: "0s", want: DefaultCommandIncludeTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inc := Include{Command: []string{"echo"}, Dest: "docs/", Timeout: tt.timeout}
			if got := inc.ResolvedTimeout(); got != tt.want {
				t.Errorf("ResolvedTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkillManifestValidate(t *testing.T) {
	tests := []struct {
		name     string
		manifest SkillManifest
		wantErr  bool
	}{
		{
			name: "valid manifest",
			manifest: SkillManifest{
				Include: []Include{
					{From: "rad", Path: "SYNTAX.md", Dest: "references/"},
					{URL: "https://example.com/file.md", Dest: "docs/file.md"},
				},
			},
			wantErr: false,
		},
		{
			name:     "empty manifest",
			manifest: SkillManifest{},
			wantErr:  false,
		},
		{
			name: "invalid include in manifest",
			manifest: SkillManifest{
				Include: []Include{
					{From: "rad", Dest: "references/"}, // Missing path
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SkillManifest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadSkillManifest(t *testing.T) {
	t.Run("valid manifest", func(t *testing.T) {
		dir := t.TempDir()
		manifestContent := `include:
  - from: rad
    path: SYNTAX.md
    dest: references/
  - url: https://example.com/file.md
    dest: docs/file.md
`
		if err := os.WriteFile(filepath.Join(dir, "tai.yaml"), []byte(manifestContent), 0644); err != nil {
			t.Fatal(err)
		}

		manifest, err := LoadSkillManifest(dir)
		if err != nil {
			t.Fatalf("LoadSkillManifest() error = %v", err)
		}
		if manifest == nil {
			t.Fatal("LoadSkillManifest() returned nil")
			return
		}
		if len(manifest.Include) != 2 {
			t.Errorf("len(manifest.Include) = %d, want 2", len(manifest.Include))
		}

		// Check first include
		if manifest.Include[0].From != "rad" {
			t.Errorf("Include[0].From = %q, want 'rad'", manifest.Include[0].From)
		}
		if manifest.Include[0].Path != "SYNTAX.md" {
			t.Errorf("Include[0].Path = %q, want 'SYNTAX.md'", manifest.Include[0].Path)
		}

		// Check second include
		if manifest.Include[1].URL != "https://example.com/file.md" {
			t.Errorf("Include[1].URL = %q, want 'https://example.com/file.md'", manifest.Include[1].URL)
		}
	})

	t.Run("no manifest returns nil", func(t *testing.T) {
		dir := t.TempDir()

		manifest, err := LoadSkillManifest(dir)
		if err != nil {
			t.Fatalf("LoadSkillManifest() error = %v", err)
		}
		if manifest != nil {
			t.Error("LoadSkillManifest() should return nil for missing manifest")
		}
	})

	t.Run("invalid yaml returns error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "tai.yaml"), []byte("invalid: [yaml"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadSkillManifest(dir)
		if err == nil {
			t.Error("LoadSkillManifest() should return error for invalid YAML")
		}
	})

	t.Run("invalid manifest returns error", func(t *testing.T) {
		dir := t.TempDir()
		manifestContent := `include:
  - from: rad
    dest: references/  # Missing path
`
		if err := os.WriteFile(filepath.Join(dir, "tai.yaml"), []byte(manifestContent), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadSkillManifest(dir)
		if err == nil {
			t.Error("LoadSkillManifest() should return error for invalid manifest")
		}
	})
}
