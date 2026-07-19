package patch

// Status is how a patch stands against the current pristine upstream.
type Status int

const (
	StatusOK              Status = iota // applies cleanly; upstream matches the blessed snapshot
	StatusDrifted                       // applies cleanly, but upstream differs from the snapshot
	StatusUnblessed                     // applies cleanly; no snapshot to compare against
	StatusBroken                        // an edit could not be applied
	StatusFileMissing                   // the target file is absent from the source
	StatusSourceUnfetched               // command source not fetched yet
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

// Inspect evaluates a patch against the current pristine content of its target.
//
// Deliberately against *pristine*, not against the patched view: comparing a
// blessed snapshot to tai's own output would report drift on every install, and
// re-applying a patch to already-patched content would fail exactly-once
// matching and report every healthy patch as broken.
func Inspect(pristine []byte, p *Patch) (*Inspection, error) {
	insp := &Inspection{Patch: p, Pristine: pristine}

	// Each patch is checked on its own against pristine, even when several
	// target one file: they are each blessed against pristine, so that is the
	// baseline each one has to fit.
	if _, applyErr := Apply(pristine, p); applyErr != nil {
		insp.Status, insp.ApplyErr = StatusBroken, applyErr
		return insp, nil
	}

	blessed, hasBlessed, err := p.LoadBlessed()
	if err != nil {
		return nil, err
	}
	insp.Blessed, insp.HasBlessed = blessed, hasBlessed

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
