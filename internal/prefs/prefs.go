// Package prefs stores machine-scoped answers to questions tai should only
// ask once.
//
// These are deliberately separate from the config repo. A config repo is
// shared between machines and checked into version control; a decision like
// "yes, you may rewrite this file on this computer" is about the machine in
// front of you, and belongs beside the manifests rather than beside the assets.
package prefs

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Filename is the prefs file's name within the tai config directory.
const Filename = "preferences.yaml"

// Prefs holds remembered answers.
type Prefs struct {
	// ConfigWriteConsent records whether tai may write a config file whose
	// terms the user was asked to agree to, keyed by the request's Key.
	ConfigWriteConsent map[string]bool `yaml:"config_write_consent,omitempty"`
}

// Store reads and writes the prefs file in a directory.
type Store struct {
	dir string
}

// NewStore creates a store rooted at a tai config directory.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Path returns the prefs file path.
func (s *Store) Path() string {
	return filepath.Join(s.dir, Filename)
}

// Load reads the prefs file. A missing file is not an error - it just means
// nothing has been decided yet.
func (s *Store) Load() (*Prefs, error) {
	data, err := os.ReadFile(s.Path())
	if os.IsNotExist(err) {
		return &Prefs{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading preferences: %w", err)
	}

	var p Prefs
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.Path(), err)
	}
	return &p, nil
}

// Save writes the prefs file, creating its directory if needed.
func (s *Store) Save(p *Prefs) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return fmt.Errorf("creating preferences directory: %w", err)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("serializing preferences: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, Filename+".tai-*")
	if err != nil {
		return fmt.Errorf("creating temp preferences file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing preferences: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp preferences file: %w", err)
	}
	if err := os.Rename(tmpName, s.Path()); err != nil {
		return fmt.Errorf("replacing preferences: %w", err)
	}
	return nil
}

// ConsentFor returns a remembered answer, and whether one was recorded.
func (p *Prefs) ConsentFor(key string) (granted bool, recorded bool) {
	granted, recorded = p.ConfigWriteConsent[key]
	return granted, recorded
}

// RecordConsent remembers an answer.
func (p *Prefs) RecordConsent(key string, granted bool) {
	if p.ConfigWriteConsent == nil {
		p.ConfigWriteConsent = make(map[string]bool)
	}
	p.ConfigWriteConsent[key] = granted
}
