package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amterp/tai/internal/testutil"
)

func TestParseTaiConfig(t *testing.T) {
	env := testutil.NewTestEnv(t)

	tests := []struct {
		name            string
		content         string
		wantErr         bool
		wantErrContains string // Substring the error must carry, when set
		check           func(*testing.T, *TaiConfig)
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
			// Removed rather than ignored: it silently appended "/assets" to
			// the path, and a config still carrying it would resolve every
			// asset one directory off without saying so.
			name: "tai-repo source is rejected with a migration hint",
			content: `version: 1
sources:
  - name: personal
    type: tai-repo
    path: ~/personal-tai-config
`,
			wantErr:        true,
			wantErrContains: "assets/",
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
		{
			name: "valid command source",
			content: `version: 1
sources:
  - name: private
    type: command
    command: [sh, -c, 'git clone repo "$TAI_DEST_DIR"']
    timeout: 2m
`,
			check: func(t *testing.T, c *TaiConfig) {
				src := c.Sources[1] // implicit local + private
				if !src.IsCommand() {
					t.Errorf("IsCommand() = false, want true")
				}
				if got := src.ResolvedTimeout(); got != 2*time.Minute {
					t.Errorf("ResolvedTimeout() = %v, want 2m", got)
				}
			},
		},
		{
			name: "command source defaults timeout",
			content: `version: 1
sources:
  - name: private
    type: command
    command: [./fetch.sh]
`,
			check: func(t *testing.T, c *TaiConfig) {
				if got := c.Sources[1].ResolvedTimeout(); got != DefaultCommandTimeout {
					t.Errorf("ResolvedTimeout() = %v, want default %v", got, DefaultCommandTimeout)
				}
			},
		},
		{
			name: "command source with path rejected",
			content: `version: 1
sources:
  - name: private
    type: command
    command: [./fetch.sh]
    path: ./assets
`,
			wantErr: true,
		},
		{
			name: "command source without command rejected",
			content: `version: 1
sources:
  - name: private
    type: command
`,
			wantErr: true,
		},
		{
			name: "command source with empty argv element rejected",
			content: `version: 1
sources:
  - name: private
    type: command
    command: [sh, '', script]
`,
			wantErr: true,
		},
		{
			name: "command source with bad timeout rejected",
			content: `version: 1
sources:
  - name: private
    type: command
    command: [./fetch.sh]
    timeout: banana
`,
			wantErr: true,
		},
		{
			name: "command on local source rejected",
			content: `version: 1
sources:
  - name: local
    path: ./assets
    command: [./fetch.sh]
`,
			wantErr: true,
		},
		{
			name: "valid url source",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://raw.githubusercontent.com/foo/bar/main/GUIDE.md
    timeout: 1m
`,
			check: func(t *testing.T, c *TaiConfig) {
				src := c.Sources[1] // implicit local + upstream
				if !src.IsURL() {
					t.Error("IsURL() = false, want true")
				}
				if !src.NeedsFetch() {
					t.Error("NeedsFetch() = false, want true")
				}
				if src.IsCommand() {
					t.Error("IsCommand() = true, want false")
				}
				if got := src.ResolvedFilename(); got != "GUIDE.md" {
					t.Errorf("ResolvedFilename() = %q, want %q", got, "GUIDE.md")
				}
				if got := src.ResolvedTimeout(); got != time.Minute {
					t.Errorf("ResolvedTimeout() = %v, want 1m", got)
				}
			},
		},
		{
			name: "url source defaults timeout",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://example.com/GUIDE.md
`,
			check: func(t *testing.T, c *TaiConfig) {
				if got := c.Sources[1].ResolvedTimeout(); got != DefaultCommandTimeout {
					t.Errorf("ResolvedTimeout() = %v, want default %v", got, DefaultCommandTimeout)
				}
			},
		},
		{
			name: "url source explicit filename overrides basename",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://example.com/raw?id=42
    filename: GUIDE.md
`,
			check: func(t *testing.T, c *TaiConfig) {
				if got := c.Sources[1].ResolvedFilename(); got != "GUIDE.md" {
					t.Errorf("ResolvedFilename() = %q, want %q", got, "GUIDE.md")
				}
			},
		},
		{
			name: "url source without url rejected",
			content: `version: 1
sources:
  - name: upstream
    type: url
`,
			wantErr: true,
		},
		{
			name: "url source with non-http scheme rejected",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: file:///etc/passwd
`,
			wantErr: true,
		},
		{
			name: "url source with path rejected",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://example.com/GUIDE.md
    path: ./assets
`,
			wantErr: true,
		},
		{
			name: "url source with command rejected",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://example.com/GUIDE.md
    command: [./fetch.sh]
`,
			wantErr: true,
		},
		{
			name: "url source with bad timeout rejected",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://example.com/GUIDE.md
    timeout: banana
`,
			wantErr: true,
		},
		{
			name: "url source with underivable filename rejected",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://example.com/
`,
			wantErr: true,
		},
		{
			name: "url source with path separator in filename rejected",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://example.com/GUIDE.md
    filename: ../escape.md
`,
			wantErr: true,
		},
		{
			name: "url on local source rejected",
			content: `version: 1
sources:
  - name: local
    path: ./assets
    url: https://example.com/GUIDE.md
`,
			wantErr: true,
		},
		{
			name: "url on command source rejected",
			content: `version: 1
sources:
  - name: private
    type: command
    command: [./fetch.sh]
    url: https://example.com/GUIDE.md
`,
			wantErr: true,
		},
		{
			name: "filename on local source rejected",
			content: `version: 1
sources:
  - name: local
    path: ./assets
    filename: GUIDE.md
`,
			wantErr: true,
		},
		{
			name: "url source can declare patches",
			content: `version: 1
sources:
  - name: upstream
    type: url
    url: https://example.com/GUIDE.md
    patches: patches/upstream/
`,
			check: func(t *testing.T, c *TaiConfig) {
				if !c.Sources[1].HasPatches() {
					t.Error("HasPatches() = false, want true")
				}
			},
		},
		{
			name: "timeout on local source rejected",
			content: `version: 1
sources:
  - name: local
    path: ./assets
    timeout: 1m
`,
			wantErr: true,
		},
		{
			name: "source name with path separator rejected",
			content: `version: 1
sources:
  - name: ../evil
    type: command
    command: [./fetch.sh]
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
				} else if tt.wantErrContains != "" && !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("error %q should mention %q", err, tt.wantErrContains)
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
			// Both entry shapes coexist in one list: nearly every entry is a
			// bare name, and requiring `name:` on all of them to allow
			// exclusions on one would be a poor trade.
			name: "bundles list mixes bare names and mappings",
			content: `name: mixed
bundles:
  - core
  - work:work-tools
  - name: work:extras
    exclude:
      - assets/skills/beagle
`,
			check: func(t *testing.T, b *Bundle) {
				want := []BundleEntry{
					{Name: "core"},
					{Name: "work:work-tools"},
					{Name: "work:extras", Exclude: []string{"assets/skills/beagle"}},
				}
				if len(b.Bundles) != len(want) {
					t.Fatalf("len(bundles) = %d, want %d", len(b.Bundles), len(want))
				}
				for i, w := range want {
					got := b.Bundles[i]
					if got.Name != w.Name {
						t.Errorf("bundles[%d].Name = %q, want %q", i, got.Name, w.Name)
					}
					if strings.Join(got.Exclude, ",") != strings.Join(w.Exclude, ",") {
						t.Errorf("bundles[%d].Exclude = %v, want %v", i, got.Exclude, w.Exclude)
					}
				}
			},
		},
		{
			name: "bundle reference naming a source but no bundle",
			content: `name: broken
bundles:
  - work:
`,
			wantErr: true,
		},
		{
			name: "bundle exclusion with an interior ** is rejected",
			content: `name: broken
bundles:
  - name: work:tools
    exclude:
      - assets/**/beagle
`,
			wantErr: true,
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
		name            string
		content         string
		wantErr         bool
		wantErrContains string // Substring the error must carry, when set
		check           func(*testing.T, *Profile)
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
			name: "profile with post_install hook",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - sh
  - -c
  - ./notify.sh
post_install_timeout: 1m
`,
			check: func(t *testing.T, p *Profile) {
				if !p.PostInstall.IsShorthand() {
					t.Fatalf("post_install = %+v, want one unnamed step", p.PostInstall)
				}
				if got := p.PostInstall[0].Command; len(got) != 3 || got[2] != "./notify.sh" {
					t.Errorf("command = %v, want the whole argv", got)
				}
				if got := p.PostInstall[0].ResolvedTimeout(p.PostInstallTimeout); got != time.Minute {
					t.Errorf("ResolvedTimeout() = %v, want 1m", got)
				}
			},
		},
		{
			name: "post_install defaults timeout",
			content: `name: work
target: claude
bundles:
  - core
post_install: [./notify.sh]
`,
			check: func(t *testing.T, p *Profile) {
				if got := p.PostInstall[0].ResolvedTimeout(p.PostInstallTimeout); got != DefaultCommandTimeout {
					t.Errorf("ResolvedTimeout() = %v, want default %v", got, DefaultCommandTimeout)
				}
			},
		},
		{
			name: "post_install named steps",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - name: memri
    command: [memri, setup]
  - name: render
    command: [./render.rad]
    timeout: 2m
post_install_timeout: 1m
`,
			check: func(t *testing.T, p *Profile) {
				if len(p.PostInstall) != 2 || p.PostInstall.IsShorthand() {
					t.Fatalf("post_install = %+v, want two named steps", p.PostInstall)
				}
				memri, render := p.PostInstall[0], p.PostInstall[1]
				if memri.Name != "memri" || len(memri.Command) != 2 {
					t.Errorf("step 0 = %+v, want memri", memri)
				}
				if got := memri.ResolvedTimeout(p.PostInstallTimeout); got != time.Minute {
					t.Errorf("memri timeout = %v, want the profile's 1m", got)
				}
				if got := render.ResolvedTimeout(p.PostInstallTimeout); got != 2*time.Minute {
					t.Errorf("render timeout = %v, want its own 2m", got)
				}
			},
		},
		{
			name: "post_install step without name rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - name: memri
    command: [memri, setup]
  - command: [./render.rad]
`,
			wantErrContains: "post_install[1]: missing 'name'",
		},
		{
			name: "post_install single step without name rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - command: [./render.rad]
`,
			wantErrContains: "post_install[0]: missing 'name'",
		},
		{
			name: "post_install duplicate step names rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - name: setup
    command: [memri, setup]
  - name: setup
    command: [./render.rad]
`,
			wantErrContains: "duplicate step name",
		},
		{
			name: "post_install step without command rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - name: setup
`,
			wantErrContains: "missing 'command'",
		},
		{
			name: "post_install step with bad timeout rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - name: setup
    command: [memri, setup]
    timeout: soon
`,
			wantErrContains: `post_install step "setup": 'timeout'`,
		},
		{
			name: "post_install step with unknown field rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - name: setup
    command: [memri, setup]
    timout: 2m
`,
			wantErrContains: `unknown post_install step field "timout"`,
		},
		{
			name: "post_install mixing argv and steps rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install:
  - ./notify.sh
  - name: setup
    command: [memri, setup]
`,
			wantErrContains: "mixes argv strings with steps",
		},
		{
			name: "post_install as a bare string rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install: ./notify.sh
`,
			wantErrContains: "'post_install' must be a list",
		},
		{
			name: "post_install with empty argv element rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install: [sh, '']
`,
			wantErr: true,
		},
		{
			name: "post_install with bad timeout rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install: [./notify.sh]
post_install_timeout: -3s
`,
			wantErr: true,
		},
		{
			name: "post_install_timeout without post_install rejected",
			content: `name: work
target: claude
bundles:
  - core
post_install_timeout: 1m
`,
			wantErr: true,
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
			if tt.wantErr || tt.wantErrContains != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("error = %v, want it to contain %q", err, tt.wantErrContains)
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

func TestValidateDestPath(t *testing.T) {
	tests := []struct {
		name    string
		dest    string
		wantErr bool
	}{
		// Valid paths
		{"empty string (root)", "", false},
		{"simple directory", "config", false},
		{"nested directory", "config/sub", false},
		{"filename with dots", "version..1", false},
		{"filename with multiple dots", "foo..bar..baz", false},
		{"current dir explicit", ".", false},

		// Invalid paths
		{"absolute path", "/etc/passwd", true},
		{"path traversal at start", "../etc/passwd", true},
		{"path traversal in middle", "foo/../bar", true},
		{"path traversal at end", "foo/bar/..", true},
		{"just path traversal", "..", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDestPath(tt.dest)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// A wildcard does not cross `/`, so a pattern relying on that would silently
// carry the file it was meant to drop. Locked in here and rejected at parse.
func TestSourceIsExcludedDoesNotSpanSeparators(t *testing.T) {
	src := Source{Exclude: []string{"skills/*"}}

	if !src.IsExcluded("skills/rad") {
		t.Error("IsExcluded(skills/rad) = false, want true")
	}
	if !src.IsExcluded("skills/rad/SKILL.md") {
		t.Error("subtree exclusion comes from the parent walk, not the wildcard")
	}
	// Two segments deep is only excluded via its parent `skills/a`, never by
	// the wildcard itself - which is why an interior `**` is a parse error.
	if src.IsExcluded("other/a/b.md") {
		t.Error("IsExcluded(other/a/b.md) = true, want false")
	}
}

func TestValidateExcludePattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{"plain path", "personas/draft.md", false},
		{"single-segment wildcard", "skills/legacy-*", false},
		{"trailing doublestar", "docs/**", false},
		{"character class", "skills/[ab]-thing", false},
		{"interior doublestar", "skills/**/legacy.md", true},
		{"leading doublestar", "**/legacy.md", true},
		{"unclosed character class", "skills/[ab", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExcludePattern(tt.pattern)
			if tt.wantErr && err == nil {
				t.Errorf("validateExcludePattern(%q) = nil, want an error", tt.pattern)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateExcludePattern(%q) = %v, want nil", tt.pattern, err)
			}
		})
	}
}

func TestSourceIsExcluded(t *testing.T) {
	src := Source{Exclude: []string{"skills/legacy-*", "personas/draft.md", "docs/**"}}

	tests := []struct {
		path string
		want bool
	}{
		{"skills/legacy-foo", true},
		{"skills/legacy-foo/SKILL.md", true}, // excluding a dir excludes its subtree
		{"skills/legacy-foo/refs/deep/guide.md", true},
		{"skills/keeper", false},
		{"skills/legacyfoo", false}, // pattern requires the hyphen
		{"personas/draft.md", true},
		{"personas/final.md", false},
		{"docs/a/b/c.md", true}, // trailing /** is accepted and stripped
		{"docs", true},
		{"other/thing.md", false},
	}

	for _, tt := range tests {
		if got := src.IsExcluded(tt.path); got != tt.want {
			t.Errorf("IsExcluded(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestSourceNoExcludesMatchesNothing(t *testing.T) {
	var src Source
	if src.IsExcluded("anything/at/all.md") {
		t.Error("a source without exclude patterns must exclude nothing")
	}
}

func TestValidateSourcePatchesAndExclude(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{"valid", "    patches: patches/shared/\n", ""},
		{"absolute patches", "    patches: /etc/patches\n", "cannot be an absolute path"},
		{"traversal patches", "    patches: ../../elsewhere\n", "path traversal"},
		{"valid exclude", "    exclude:\n      - \"skills/legacy-*\"\n", ""},
		{"empty exclude", "    exclude:\n      - \"\"\n", "is empty"},
		{"absolute exclude", "    exclude:\n      - \"/etc/*\"\n", "cannot be an absolute path"},
		{"traversal exclude", "    exclude:\n      - \"../secrets\"\n", "path traversal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.NewTestEnv(t)
			env.SetupTaiConfig("version: 1\nsources:\n  - name: shared\n    path: ./shared\n" + tt.source)

			_, err := ParseTaiConfig(filepath.Join(env.ConfigRepo, "tai.yaml"))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
