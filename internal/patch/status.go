package patch

// Status is how a patch stands against the current pristine upstream.
type Status int

const (
	StatusOK              Status = iota // applies cleanly; upstream matches the blessed snapshot
	StatusDrifted                       // applies cleanly, but upstream differs from the snapshot
	StatusUnblessed                     // applies cleanly; no snapshot to compare against
	StatusBroken                        // an edit could not be applied
	StatusFileMissing                   // the target file is absent from the source
	StatusSourceUnfetched               // a fetched source (command or url) whose fetch has not run
)

// String renders the status as `tai patch list` shows it.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusDrifted:
		return "ok, upstream drifted"
	case StatusUnblessed:
		return "unblessed"
	case StatusBroken:
		return "BROKEN"
	case StatusFileMissing:
		return "target file missing"
	case StatusSourceUnfetched:
		return "source not fetched"
	}
	return "unknown"
}

// Inspection is the full picture of one patch: whether it still applies, and
// the content on both sides of the comparison.
type Inspection struct {
	Patch      *Patch
	Status     Status
	ApplyErr   *ApplyError // set only when Status is StatusBroken
	Pristine   []byte      // current upstream content (nil when unreadable)
	Blessed    []byte      // snapshot content (nil when unblessed)
	HasBlessed bool
}

// Inspect evaluates a patch against current upstream.
//
// The two content arguments are deliberately distinct, because the two
// questions this answers have different baselines:
//
//   - "does it still apply?" is asked against baseline - pristine with any
//     earlier patches on the same file already folded in, exactly as the
//     install pipeline applies them. Checking each patch against raw pristine
//     instead would report a patch that edits text an earlier sibling
//     introduced as broken, when it installs perfectly well.
//   - "has upstream moved?" is asked against pristine, never the view or a
//     folded baseline: a blessed snapshot is a record of upstream, so
//     comparing it to content tai itself produced would report drift forever.
//
// With one patch on a file the two are identical.
func Inspect(pristine, baseline []byte, p *Patch) (*Inspection, error) {
	insp := &Inspection{Patch: p, Pristine: pristine}

	// Loaded before the apply check, and regardless of its outcome. A broken
	// patch is exactly when the snapshot earns its keep: "what changed upstream
	// since I last looked" is the first question asked during a repair, and it
	// does not need the patch to still apply in order to be answerable.
	blessed, hasBlessed, err := p.LoadBlessed()
	if err != nil {
		return nil, err
	}
	insp.Blessed, insp.HasBlessed = blessed, hasBlessed

	if _, applyErr := Apply(baseline, p); applyErr != nil {
		insp.Status, insp.ApplyErr = StatusBroken, applyErr
		return insp, nil
	}

	switch {
	case !hasBlessed:
		insp.Status = StatusUnblessed
	case string(blessed) != string(pristine):
		insp.Status = StatusDrifted
	default:
		insp.Status = StatusOK
	}
	return insp, nil
}

// MissingInspection reports a patch whose target could not be read.
func MissingInspection(p *Patch, status Status) *Inspection {
	return &Inspection{Patch: p, Status: status}
}
