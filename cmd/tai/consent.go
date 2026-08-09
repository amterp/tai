package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/amterp/radish"

	"github.com/amterp/tai/internal/configasset"
	"github.com/amterp/tai/internal/output"
	"github.com/amterp/tai/internal/prefs"
)

// Consent answers, as shown in the prompt.
const (
	consentAllowRemember = "Allow, and remember for this machine"
	consentAllowOnce     = "Allow just this once"
	consentDecline       = "Decline"
)

// consentResolver answers a config-write consent request: from a remembered
// answer, from the user, or - when nothing can be asked - by declining with an
// explanation of how to proceed.
//
// This is the only interactive part of tai. Everything else is scriptable, and
// keeping the prompt confined here is what keeps it that way.
type consentResolver struct {
	out      output.Output
	store    *prefs.Store
	prefs    *prefs.Prefs
	preAllow bool // --allow-config-rewrite: allow without asking, once
}

func newConsentResolver(out output.Output, configDir string, preAllow bool) (*consentResolver, error) {
	store := prefs.NewStore(configDir)
	loaded, err := store.Load()
	if err != nil {
		return nil, err
	}
	return &consentResolver{out: out, store: store, prefs: loaded, preAllow: preAllow}, nil
}

// Resolve implements the installer's consent callback.
func (r *consentResolver) Resolve(req configasset.ConsentRequest) (bool, error) {
	if r.preAllow {
		return true, nil
	}
	if granted, recorded := r.prefs.ConsentFor(req.Key); recorded {
		if !granted {
			// The explainer and the "change it by editing..." note printed once,
			// when the question was answered - possibly weeks ago, possibly to
			// someone else. Without this, every later run reports a refusal with
			// no way back.
			r.out.Info("Using a remembered answer: no. Change it by editing " +
				r.store.Path() + ", or override once with --allow-config-rewrite.")
		}
		return granted, nil
	}

	r.explain(req)

	choice, ok, err := radish.RunSelect(
		radish.NewSelect().
			Title(req.Question).
			Options(consentAllowRemember, consentAllowOnce, consentDecline),
		os.Stdin, os.Stderr)

	switch {
	case errors.Is(err, radish.ErrNotInteractive):
		// Nothing to ask, and guessing either way is wrong: silently declining
		// looks like a bug, silently allowing writes a file behind the user's
		// back. Decline, and say exactly how to proceed.
		r.out.Warning("Not an interactive terminal, so this cannot be asked - declining for now.")
		r.out.Info("To allow it: re-run with --allow-config-rewrite, or answer once in an interactive terminal.")
		return false, nil
	case err != nil:
		return false, fmt.Errorf("asking for consent: %w", err)
	case !ok:
		// Cancelled with Esc or Ctrl-C.
		return false, nil
	}

	granted := choice != consentDecline
	if choice == consentAllowRemember || choice == consentDecline {
		// A decline is remembered too - otherwise every install re-asks a
		// question already answered.
		r.prefs.RecordConsent(req.Key, granted)
		if err := r.store.Save(r.prefs); err != nil {
			return granted, fmt.Errorf("saving preferences: %w", err)
		}
		r.out.Info("Remembered. Change it by editing " + r.store.Path() + ".")
	}

	return granted, nil
}

// explain prints the request's details before the prompt, since a Select title
// is a single line.
func (r *consentResolver) explain(req configasset.ConsentRequest) {
	r.out.Blank()
	for _, line := range req.Details {
		if strings.TrimSpace(line) == "" {
			r.out.Blank()
			continue
		}
		r.out.Raw("  " + line)
	}
	r.out.Blank()
}
