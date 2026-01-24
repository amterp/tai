package config

import (
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/testutil"
)

func TestParseTaiConfig(t *testing.T) {
	env := testutil.NewTestEnv(t)

	tests := []struct {
		name    string
		content string
		wantErr bool
		check   func(*testing.T, *TaiConfig)
	}{
		{
			name: "valid minimal config",
			content: `version: 1
sources:
  - name: local
    path: ./assets
`,
			check: func(t *testing.T, c *TaiConfig) {
				if c.Version != 1 {
					t.Errorf("version = %d, want 1", c.Version)
				}
				if len(c.Sources) != 1 {
					t.Errorf("len(sources) = %d, want 1", len(c.Sources))
				}
				if c.Sources[0].Name != "local" {
					t.Errorf("source.name = %q, want 'local'", c.Sources[0].Name)
				}
			},
		},
		{
			name: "config with implicit local source",
			content: `version: 1
sources:
  - name: work
    path: ~/work/config
`,
			check: func(t *testing.T, c *TaiConfig) {
				// Should have implicit local source added at beginning
				if len(c.Sources) != 2 {
					t.Errorf("len(sources) = %d, want 2", len(c.Sources))
				}
				if c.Sources[0].Name != "local" {
					t.Errorf("first source = %q, want 'local'", c.Sources[0].Name)
				}
				if c.Sources[1].Name != "work" {
					t.Errorf("second source = %q, want 'work'", c.Sources[1].Name)
				}
			},
		},
		{
			name: "config with tai-repo source",
			content: `version: 1
sources:
  - name: personal
    type: tai-repo
    path: ~/personal-tai-config
`,
			check: func(t *testing.T, c *TaiConfig) {
				if len(c.Sources) != 2 { // implicit local + personal
					t.Errorf("len(sources) = %d, want 2", len(c.Sources))
				}
				if c.Sources[1].Type != "tai-repo" {
					t.Errorf("source.type = %q, want 'tai-repo'", c.Sources[1].Type)
				}
			},
		},
		{
			name: "invalid version",
			content: `version: 2
sources:
  - name: local
    path: ./assets
`,
			wantErr: true,
		},
		{
			name: "duplicate source name",
			content: `version: 1
sources:
  - name: local
    path: ./assets
  - name: local
    path: ./other
`,
			wantErr: true,
		},
		{
			name: "missing source path",
			content: `version: 1
sources:
  - name: local
`,
			wantErr: true,
		},
		{
			name: "invalid source type",
			content: `version: 1
sources:
  - name: local
    type: invalid
    path: ./assets
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(env.Root, "test-tai.yaml")
			env.WriteFile("test-tai.yaml", tt.content)

			config, err := ParseTaiConfig(path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}

func TestParseBundle(t *testing.T) {
	env := testutil.NewTestEnv(t)

	tests := []struct {
		name    string
		content string
		wantErr bool
		check   func(*testing.T, *Bundle)
	}{
		{
			name: "valid bundle with assets",
			content: `name: core
description: Core assets
assets:
  - type: persona
    path: personas/senior-swe.md
  - type: delegate
    path: delegates/code-reviewer.md
`,
			check: func(t *testing.T, b *Bundle) {
				if b.Name != "core" {
					t.Errorf("name = %q, want 'core'", b.Name)
				}
				if len(b.Assets) != 2 {
					t.Errorf("len(assets) = %d, want 2", len(b.Assets))
				}
			},
		},
		{
			name: "bundle with included bundles",
			content: `name: full-setup
description: Complete setup
bundles:
  - core
  - extras
assets:
  - type: persona
    path: personas/custom.md
`,
			check: func(t *testing.T, b *Bundle) {
				if len(b.Bundles) != 2 {
					t.Errorf("len(bundles) = %d, want 2", len(b.Bundles))
				}
				if len(b.Assets) != 1 {
					t.Errorf("len(assets) = %d, want 1", len(b.Assets))
				}
			},
		},
		{
			name: "bundle with source prefix",
			content: `name: work
assets:
  - type: persona
    path: work-shared:personas/company-style.md
`,
			check: func(t *testing.T, b *Bundle) {
				if b.Assets[0].Path != "work-shared:personas/company-style.md" {
					t.Errorf("asset.Path = %q, want 'work-shared:personas/company-style.md'", b.Assets[0].Path)
				}
				if b.Assets[0].Type != "persona" {
					t.Errorf("asset.Type = %q, want 'persona'", b.Assets[0].Type)
				}
			},
		},
		{
			name:    "missing name",
			content: `description: No name`,
			wantErr: true,
		},
		{
			name: "missing asset type",
			content: `name: bad
assets:
  - path: personas/test.md
`,
			wantErr: true,
		},
		{
			name: "missing asset path",
			content: `name: bad
assets:
  - type: persona
`,
			wantErr: true,
		},
		{
			name: "invalid asset type",
			content: `name: bad
assets:
  - type: invalid
    path: personas/test.md
`,
			wantErr: true,
		},
		{
			name: "old string-based asset format gives helpful error",
			content: `name: legacy
assets:
  - skills/my-skill
  - personas/senior-swe.md
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(env.Root, "test-bundle.yaml")
			env.WriteFile("test-bundle.yaml", tt.content)

			bundle, err := ParseBundle(path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, bundle)
			}
		})
	}
}

func TestParseProfile(t *testing.T) {
	env := testutil.NewTestEnv(t)

	tests := []struct {
		name    string
		content string
		wantErr bool
		check   func(*testing.T, *Profile)
	}{
		{
			name: "valid profile with bundles",
			content: `name: work
description: Work profile
target: claude
bundles:
  - core
  - work-extras
`,
			check: func(t *testing.T, p *Profile) {
				if p.Name != "work" {
					t.Errorf("name = %q, want 'work'", p.Name)
				}
				if p.Target != "claude" {
					t.Errorf("target = %q, want 'claude'", p.Target)
				}
				if len(p.Bundles) != 2 {
					t.Errorf("len(bundles) = %d, want 2", len(p.Bundles))
				}
			},
		},
		{
			name: "profile with direct assets",
			content: `name: minimal
target: claude
assets:
  - type: persona
    path: personas/default.md
`,
			check: func(t *testing.T, p *Profile) {
				if len(p.Assets) != 1 {
					t.Errorf("len(assets) = %d, want 1", len(p.Assets))
				}
				if p.Assets[0].Type != "persona" {
					t.Errorf("asset.Type = %q, want 'persona'", p.Assets[0].Type)
				}
			},
		},
		{
			name: "profile with options",
			content: `name: custom
target: claude
bundles:
  - core
options:
  some_option: true
  another: value
`,
			check: func(t *testing.T, p *Profile) {
				if p.Options == nil {
					t.Fatal("options is nil")
				}
				if v, ok := p.Options["some_option"]; !ok || v != true {
					t.Errorf("options[some_option] = %v, want true", v)
				}
			},
		},
		{
			name:    "missing name",
			content: `target: claude`,
			wantErr: true,
		},
		{
			name: "missing target",
			content: `name: bad
bundles:
  - core
`,
			wantErr: true,
		},
		{
			name: "no bundles or assets",
			content: `name: empty
target: claude
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(env.Root, "test-profile.yaml")
			env.WriteFile("test-profile.yaml", tt.content)

			profile, err := ParseProfile(path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, profile)
			}
		})
	}
}

func TestParseAssetReference(t *testing.T) {
	tests := []struct {
		ref        string
		wantSource string
		wantPath   string
	}{
		{"personas/swe.md", "local", "personas/swe.md"},
		{"work:personas/swe.md", "work", "personas/swe.md"},
		{"personal:delegates/reviewer.md", "personal", "delegates/reviewer.md"},
		{"source:with:colons.md", "source", "with:colons.md"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			source, path := ParseAssetReference(tt.ref)
			if source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
		})
	}
}

func TestLoadBundlesFromDir(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupBundle("core", `name: core
description: Core bundle
assets:
  - type: persona
    path: personas/default.md
`)
	env.SetupBundle("extras", `name: extras
description: Extra bundle
assets:
  - type: delegate
    path: delegates/helper.md
`)

	bundles, err := LoadBundlesFromDir(filepath.Join(env.ConfigRepo, "bundles"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bundles) != 2 {
		t.Errorf("len(bundles) = %d, want 2", len(bundles))
	}
	if _, ok := bundles["core"]; !ok {
		t.Error("missing 'core' bundle")
	}
	if _, ok := bundles["extras"]; !ok {
		t.Error("missing 'extras' bundle")
	}
}

func TestLoadProfilesFromDir(t *testing.T) {
	env := testutil.NewTestEnv(t)

	env.SetupProfile("personal", `name: personal
target: claude
bundles:
  - core
`)
	env.SetupProfile("work", `name: work
target: claude
bundles:
  - core
  - extras
`)

	profiles, err := LoadProfilesFromDir(filepath.Join(env.ConfigRepo, "profiles"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(profiles) != 2 {
		t.Errorf("len(profiles) = %d, want 2", len(profiles))
	}
	if _, ok := profiles["personal"]; !ok {
		t.Error("missing 'personal' profile")
	}
	if _, ok := profiles["work"]; !ok {
		t.Error("missing 'work' profile")
	}
}
