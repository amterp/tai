package install

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amterp/tai/internal/config"
)

const threeAllows = "## allow: A\n\nalpha\n\n## allow: B\n\nbeta\n\n## allow: C\n\ngamma\n"

const (
	allowA = "A: alpha"
	allowB = "B: beta"
	allowC = "C: gamma"
)

func installThreeAllows(t *testing.T) (func(InstallOptions) (*InstallResult, error), string) {
	t.Helper()
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("approvals", "p.md", threeAllows)
	assets := []*config.Asset{approvalAsset(env, "p.md")}
	install := func(opts InstallOptions) (*InstallResult, error) {
		return installer.Install(adap, "p", assets, opts)
	}
	if _, err := install(InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	return install, filepath.Join(env.TargetDir, "settings.json")
}

func setAutoModeAllow(t *testing.T, path string, allow []any) {
	t.Helper()
	settings := mustReadSettings(t, path)
	settings["autoMode"].(map[string]any)["allow"] = allow
	writeJSONFile(t, path, settings)
}

// An edit after an earlier element moved is still an edit: re-adding the
// original beside a rule the user narrowed would silently widen it again.
func TestEditAfterShiftConflicts(t *testing.T) {
	for name, edited := range map[string][]any{
		"defaults deleted, A edited": {"A: narrowed", allowB, allowC},
		"A deleted, C edited":        {"$defaults", allowB, "C: narrowed"},
		"user entry added, B edited": {"$defaults", "mine", allowA, "B: narrowed", allowC},
	} {
		t.Run(name, func(t *testing.T) {
			install, path := installThreeAllows(t)
			setAutoModeAllow(t, path, edited)

			_, err := install(InstallOptions{})
			var conflicts *ConflictsError
			if !errors.As(err, &conflicts) {
				t.Fatalf("err = %v, want a conflict", err)
			}
			if got := autoModeArray(t, mustReadSettings(t, path), "allow"); !slices.Equal(got, edited) {
				t.Errorf("allow = %q, want it untouched", got)
			}
		})
	}
}

// --force must not overwrite what sits at tai's old position once the array
// has shifted: that element is someone else's.
func TestForceAfterShiftKeepsUsersElements(t *testing.T) {
	install, path := installThreeAllows(t)
	setAutoModeAllow(t, path, []any{"A: narrowed", allowB, allowC})

	if _, err := install(InstallOptions{Force: true}); err != nil {
		t.Fatalf("--force failed: %v", err)
	}
	got := autoModeArray(t, mustReadSettings(t, path), "allow")
	if !slices.Contains(got, "A: narrowed") || !slices.Contains(got, allowA) || len(got) != 4 {
		t.Errorf("allow = %q, want the user's element kept and A appended", got)
	}
}

// The ordinary edit - the user's version exactly where tai's was - is taken
// back in place by --force, not duplicated beside it.
func TestForceReplacesEditAtItsOwnPosition(t *testing.T) {
	install, path := installThreeAllows(t)
	setAutoModeAllow(t, path, []any{"$defaults", allowA, "B: narrowed", allowC})

	if _, err := install(InstallOptions{Force: true}); err != nil {
		t.Fatalf("--force failed: %v", err)
	}
	want := []any{"$defaults", allowA, allowB, allowC}
	if got := autoModeArray(t, mustReadSettings(t, path), "allow"); !slices.Equal(got, want) {
		t.Errorf("allow = %q, want %q", got, want)
	}
}

func TestPlainDeletionAfterShiftRestores(t *testing.T) {
	install, path := installThreeAllows(t)
	setAutoModeAllow(t, path, []any{allowB, allowC}) // $defaults and A deleted

	result, err := install(InstallOptions{})
	if err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	if len(result.Restored) != 1 {
		t.Errorf("Restored = %v, want the asset", result.Restored)
	}
	if got := autoModeArray(t, mustReadSettings(t, path), "allow"); !slices.Equal(got, []any{allowB, allowC, allowA}) {
		t.Errorf("allow = %q, want A back once and $defaults left out as the user chose", got)
	}
}

// An entry recorded before snapshots existed has nothing to tell a deletion
// from an edit by, so a missing element is a conflict.
func TestMissingElementWithoutSnapshotConflicts(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("approvals", "p.md", threeAllows)
	assets := []*config.Asset{approvalAsset(env, "p.md")}
	if _, err := installer.Install(adap, "p", assets, InstallOptions{}); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	store := NewManifestStore(filepath.Join(env.ConfigDir, "manifests"))
	manifest, err := store.Load(adap.Name())
	if err != nil {
		t.Fatalf("loading manifest: %v", err)
	}
	for i := range manifest.ConfigEntries {
		for j := range manifest.ConfigEntries[i].Edits {
			manifest.ConfigEntries[i].Edits[j].Siblings = nil
		}
	}
	if err := store.Save(manifest); err != nil {
		t.Fatalf("saving manifest: %v", err)
	}

	setAutoModeAllow(t, filepath.Join(env.TargetDir, "settings.json"), []any{"$defaults", allowB, allowC})
	_, err = installer.Install(adap, "p", assets, InstallOptions{})
	var conflicts *ConflictsError
	if !errors.As(err, &conflicts) {
		t.Fatalf("err = %v, want a conflict", err)
	}
}

func TestApprovalFileWithoutEntriesWarns(t *testing.T) {
	env, installer, adap := setupStatusLineTest(t)
	env.SetupAsset("approvals", "typo.md", "# allow: A\n\nalpha\n")

	result, err := installer.Install(adap, "p", []*config.Asset{approvalAsset(env, "typo.md")}, InstallOptions{})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "no entries found") {
		t.Errorf("Warnings = %v, want one about no entries", result.Warnings)
	}
}

func TestApprovalParsesPastByteOrderMark(t *testing.T) {
	entries, err := config.ParseApprovalPolicy([]byte("\ufeff## allow: A\n\nalpha\n"))
	if err != nil || len(entries) != 1 || entries[0].Title != "A" {
		t.Fatalf("entries = %+v, err = %v, want the one entry", entries, err)
	}
}
