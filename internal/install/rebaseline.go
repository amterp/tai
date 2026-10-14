package install

import (
	"fmt"
	"path/filepath"

	"github.com/amterp/tai/internal/adapter"
)

// PostInstallTracker keeps the manifest baseline in step with post_install
// steps that post-process managed files - a formatter, or a tool injecting
// machine-local config. Conflict detection compares disk against the checksum
// recorded at write time, so without a rebaseline a step's own edit makes that
// file conflict on every subsequent install, forever. Edits from a step that
// succeeded are as intentional as the asset content (tai ran the step, from the
// user's own config), so they belong in the baseline; conflict detection is
// there to protect hand edits.
//
// The caller runs the steps and calls AfterStep after each one, which
// attributes every changed file to the step that just ran. A failed step's
// edits are left out of the baseline: a step that mutated a file and then
// failed leaves a partial edit, which is a conflict worth surfacing. A file a
// failed step touched stays out for the rest of the run, even if a later step
// that succeeds also edits it, since baselining it then would absorb the
// partial edit too.
//
// Only files this install wrote are tracked. An entry skipped via
// --skip-conflicts deliberately keeps its stale checksum so it keeps
// conflicting until the user resolves it - baselining that would silently
// discard a decision the user made.
type PostInstallTracker struct {
	installer *Installer
	targetDir string
	manifest  *Manifest

	// lastSeen holds each tracked path's checksum as of the previous step, so
	// a difference is attributable to the step that just ran.
	lastSeen map[string]string
	tainted  map[string]bool
}

// StepChanges lists the managed files one post_install step changed, as
// target-relative paths.
type StepChanges struct {
	// Baselined files now have their on-disk checksum recorded in the manifest.
	Baselined []string
	// Held files keep their previous baseline, so the next install reports
	// them as conflicts: the step failed, or an earlier failed step had
	// already changed them.
	Held []string
}

// TrackPostInstall starts tracking the files result's install wrote.
func (i *Installer) TrackPostInstall(adap adapter.Adapter, result *InstallResult) *PostInstallTracker {
	t := &PostInstallTracker{
		installer: i,
		targetDir: adap.TargetDir(),
		lastSeen:  make(map[string]string),
		tainted:   make(map[string]bool),
	}
	if result == nil || result.Manifest == nil {
		return t
	}
	t.manifest = result.Manifest

	written := make(map[string]bool)
	for _, group := range [][]string{result.Added, result.Restored, result.Updated, result.Unchanged} {
		for _, path := range group {
			written[path] = true
		}
	}
	for _, entry := range result.Manifest.Files {
		if written[entry.Path] {
			t.lastSeen[entry.Path] = entry.Checksum
		}
	}
	return t
}

// AfterStep re-hashes the tracked files, attributes any change to the step
// that just ran, and saves the manifest if any change was baselined.
func (t *PostInstallTracker) AfterStep(succeeded bool) (StepChanges, error) {
	var changes StepChanges
	if t.manifest == nil {
		return changes, nil
	}

	for idx := range t.manifest.Files {
		entry := &t.manifest.Files[idx]
		previous, tracked := t.lastSeen[entry.Path]
		if !tracked {
			continue
		}

		fullPath := filepath.Join(t.targetDir, entry.Path)
		var checksum string
		var err error
		if entry.IsDir {
			checksum, err = ComputeDirChecksum(fullPath)
		} else {
			checksum, err = ComputeFileChecksum(fullPath)
		}
		if err != nil {
			// A file a step removed is not a conflict - checkConflict treats a
			// missing path as "install it again" - so keep the recorded checksum
			// and carry on rather than failing the rebaseline for the others.
			t.installer.logger.Printf("Could not re-checksum %s after a post_install step (%v); "+
				"keeping the recorded checksum - the next install will reinstall it", entry.Path, err)
			continue
		}
		if checksum == previous {
			continue
		}
		t.lastSeen[entry.Path] = checksum

		if !succeeded {
			t.tainted[entry.Path] = true
		}
		if t.tainted[entry.Path] {
			changes.Held = append(changes.Held, entry.Path)
			continue
		}
		entry.Checksum = checksum
		changes.Baselined = append(changes.Baselined, entry.Path)
	}

	if len(changes.Baselined) == 0 {
		return changes, nil
	}
	if err := t.installer.manifestStore.Save(t.manifest); err != nil {
		return changes, fmt.Errorf("saving rebaselined manifest: %w", err)
	}
	return changes, nil
}
