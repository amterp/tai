package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amterp/tai/internal/adapter/claude"
	"github.com/amterp/tai/internal/config"
	"github.com/amterp/tai/internal/install"
	"github.com/amterp/tai/internal/profile"
	"github.com/amterp/tai/internal/source"
	"github.com/amterp/tai/internal/testutil"
)

// importEnv is a consumer config repo plus a producer repo it imports bundles
// from, wired up the way a real two-repo setup is.
type importEnv struct {
	*testutil.TestEnv
	producer string
}

func (e *importEnv) writeProducerFile(t *testing.T, rel, content string) {
	t.Helper()
	path := filepath.Join(e.producer, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// installProfile resolves and installs a profile, reloading the config repo
// from disk each time so a test can edit the producer between installs.
func (e *importEnv) installProfile(t *testing.T, name string) *install.InstallResult {
	t.Helper()

	taiConfig, err := config.ParseTaiConfig(filepath.Join(e.ConfigRepo, "tai.yaml"))
	if err != nil {
		t.Fatalf("parsing tai.yaml: %v", err)
	}
	sources, err := source.NewLoader(e.ConfigRepo).LoadAll(taiConfig.Sources)
	if err != nil {
		t.Fatalf("loading sources: %v", err)
	}
	bundles, err := config.LoadBundlesFromDir(filepath.Join(e.ConfigRepo, "bundles"))
	if err != nil {
		t.Fatalf("loading bundles: %v", err)
	}
	profiles, err := config.LoadProfilesFromDir(filepath.Join(e.ConfigRepo, "profiles"))
	if err != nil {
		t.Fatalf("loading profiles: %v", err)
	}

	resolved, err := profile.NewResolver(profiles, bundles, sources).Resolve(name)
	if err != nil {
		t.Fatalf("resolving profile %q: %v", name, err)
	}

	manifestStore := install.NewManifestStore(filepath.Join(e.ConfigDir, "manifests"))
	installer := install.NewInstaller(manifestStore, e.ConfigRepo, sources)
	result, err := installer.Install(claude.New(e.TargetDir), name, resolved.Assets, install.InstallOptions{})
	if err != nil {
		t.Fatalf("installing %q: %v", name, err)
	}
	return result
}

func skill(name string) string {
	return `---
tai-schema-version: 1
name: ` + name + `
description: The ` + name + ` skill
---

# ` + name + `
`
}

// setupImportEnv builds a producer repo offering a bundle, and a consumer that
// imports it by name rather than mirroring its contents.
func setupImportEnv(t *testing.T) *importEnv {
	t.Helper()
	base := testutil.NewTestEnv(t)
	env := &importEnv{TestEnv: base, producer: filepath.Join(base.Root, "work-tai-config")}

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: work
    path: ` + env.producer + `
default_profile: dev
`)

	env.writeProducerFile(t, "assets/skills/dredge/SKILL.md", skill("dredge"))
	env.writeProducerFile(t, "assets/skills/beagle/SKILL.md", skill("beagle"))
	env.writeProducerFile(t, "bundles/work-tools.yaml", `name: work-tools
description: Work tooling
assets:
  - type: skill
    path: assets/skills/dredge
  - type: skill
    path: assets/skills/beagle
`)

	env.SetupProfile("dev", `name: dev
description: Dev profile
target: claude
bundles:
  - work:work-tools
`)

	return env
}

// The regression this feature exists for: a skill added to the producer's
// bundle must reach the consumer's install without the consumer editing
// anything. Under the old mirror-by-hand approach it silently did not.
func TestE2EImportedBundleTracksUpstreamEdits(t *testing.T) {
	env := setupImportEnv(t)

	env.installProfile(t, "dev")
	for _, name := range []string{"dredge", "beagle"} {
		if !env.TargetFileExists(filepath.Join("skills", name, "SKILL.md")) {
			t.Fatalf("skills/%s/SKILL.md not installed", name)
		}
	}

	// Upstream adds a skill to its bundle. The consumer's repo is untouched.
	env.writeProducerFile(t, "assets/skills/recl/SKILL.md", skill("recl"))
	env.writeProducerFile(t, "bundles/work-tools.yaml", `name: work-tools
description: Work tooling
assets:
  - type: skill
    path: assets/skills/dredge
  - type: skill
    path: assets/skills/beagle
  - type: skill
    path: assets/skills/recl
`)

	env.installProfile(t, "dev")

	if !env.TargetFileExists(filepath.Join("skills", "recl", "SKILL.md")) {
		t.Error("a skill added to the imported bundle did not reach the consumer's install")
	}
}

// Machines differ, so a wholesale import needs a way to drop individual
// assets without giving up automatic updates for the rest.
func TestE2EImportedBundleHonorsPerImportExclusion(t *testing.T) {
	env := setupImportEnv(t)

	env.SetupProfile("dev", `name: dev
description: Dev profile
target: claude
bundles:
  - name: work:work-tools
    exclude:
      - assets/skills/beagle
`)

	env.installProfile(t, "dev")

	if !env.TargetFileExists(filepath.Join("skills", "dredge", "SKILL.md")) {
		t.Error("skills/dredge/SKILL.md should have installed")
	}
	if env.TargetFileExists(filepath.Join("skills", "beagle", "SKILL.md")) {
		t.Error("excluded skill was installed anyway")
	}
}

// A source-level exclusion means "never take this from that source". An
// imported bundle naming it is upstream's reference, not the consumer's, so
// the install filters it instead of failing.
func TestE2ESourceExclusionFiltersImportedBundle(t *testing.T) {
	env := setupImportEnv(t)

	env.SetupTaiConfig(`version: 1
sources:
  - name: local
    path: ./assets
  - name: work
    path: ` + env.producer + `
    exclude:
      - assets/skills/beagle
default_profile: dev
`)

	env.installProfile(t, "dev")

	if !env.TargetFileExists(filepath.Join("skills", "dredge", "SKILL.md")) {
		t.Error("skills/dredge/SKILL.md should have installed")
	}
	if env.TargetFileExists(filepath.Join("skills", "beagle", "SKILL.md")) {
		t.Error("source-excluded skill was installed anyway")
	}
}
